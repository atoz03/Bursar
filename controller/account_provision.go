package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

var provisionLocalUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type privateKeyEnvelope struct {
	Version         string `json:"v"`
	Algorithm       string `json:"alg"`
	SaltHex         string `json:"salt"`
	NonceHex        string `json:"nonce"`
	CiphertextHex   string `json:"ciphertext"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
	BillingUsername string `json:"billing_username"`
	SSHHost         string `json:"ssh_host"`
	SSHPort         int    `json:"ssh_port"`
	FileName        string `json:"file_name"`
	IssuedAt        string `json:"issued_at"`
}

func normalizeDecryptCode(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.ReplaceAll(raw, " ", "")
	return raw
}

func deriveProvisionKey(decryptCode string, salt []byte) []byte {
	normCode := normalizeDecryptCode(decryptCode)
	sum := sha256.Sum256([]byte("gpuops-provision-key-v1|" + normCode + "|" + hex.EncodeToString(salt)))
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

func buildEncryptedPrivateKeyPayload(
	privateKey string,
	decryptCode string,
	nodeID string,
	localUsername string,
	billingUsername string,
	sshHost string,
	sshPort int,
) (string, error) {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return "", fmt.Errorf("private_key 不能为空")
	}
	if !strings.Contains(privateKey, "BEGIN OPENSSH PRIVATE KEY") || !strings.Contains(privateKey, "END OPENSSH PRIVATE KEY") {
		return "", fmt.Errorf("private_key 不是 OpenSSH 私钥格式")
	}
	if normalizeDecryptCode(decryptCode) == "" {
		return "", fmt.Errorf("decrypt_code 不能为空")
	}
	if sshPort <= 0 {
		sshPort = 22
	}
	fileName := fmt.Sprintf("%d_%s.txt", sshPort, localUsername)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	key := deriveProvisionKey(decryptCode, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext := []byte(privateKey + "\n")
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	env := privateKeyEnvelope{
		Version:         "gpuops-key-v1",
		Algorithm:       "aes-256-gcm+sha256",
		SaltHex:         hex.EncodeToString(salt),
		NonceHex:        hex.EncodeToString(nonce),
		CiphertextHex:   hex.EncodeToString(ciphertext),
		NodeID:          strings.TrimSpace(nodeID),
		LocalUsername:   strings.TrimSpace(localUsername),
		BillingUsername: strings.TrimSpace(billingUsername),
		SSHHost:         strings.TrimSpace(sshHost),
		SSHPort:         sshPort,
		FileName:        fileName,
		IssuedAt:        formatRFC3339InBeijing(nowInBeijing()),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decryptProvisionPrivateKeyPayload(encryptedPayload string, decryptCode string) (string, privateKeyEnvelope, error) {
	var env privateKeyEnvelope
	encryptedPayload = strings.TrimSpace(encryptedPayload)
	if encryptedPayload == "" {
		return "", env, fmt.Errorf("encrypted_payload 不能为空")
	}
	if normalizeDecryptCode(decryptCode) == "" {
		return "", env, fmt.Errorf("decrypt_code 不能为空")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encryptedPayload)
	if err != nil {
		return "", env, fmt.Errorf("加密串不是有效 base64url: %w", err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", env, fmt.Errorf("加密串 JSON 解析失败: %w", err)
	}
	if strings.TrimSpace(env.Version) != "gpuops-key-v1" {
		return "", env, fmt.Errorf("加密串版本不支持")
	}
	salt, err := hex.DecodeString(strings.TrimSpace(env.SaltHex))
	if err != nil || len(salt) == 0 {
		return "", env, fmt.Errorf("加密串 salt 字段无效")
	}
	nonce, err := hex.DecodeString(strings.TrimSpace(env.NonceHex))
	if err != nil || len(nonce) == 0 {
		return "", env, fmt.Errorf("加密串 nonce 字段无效")
	}
	ciphertext, err := hex.DecodeString(strings.TrimSpace(env.CiphertextHex))
	if err != nil || len(ciphertext) == 0 {
		return "", env, fmt.Errorf("加密串 ciphertext 字段无效")
	}
	key := deriveProvisionKey(decryptCode, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", env, fmt.Errorf("创建解密密钥失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", env, fmt.Errorf("创建 GCM 失败: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", env, fmt.Errorf("解密失败，请检查提取码是否正确")
	}
	privateKey := strings.TrimSpace(string(plain))
	if !strings.Contains(privateKey, "BEGIN OPENSSH PRIVATE KEY") || !strings.Contains(privateKey, "END OPENSSH PRIVATE KEY") {
		return "", env, fmt.Errorf("解密结果不是有效 OpenSSH 私钥")
	}
	return privateKey + "\n", env, nil
}

func randomAlphaNum(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("n 必须 > 0")
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(out), nil
}

func randomDecryptCode() (string, error) {
	raw, err := randomAlphaNum(12)
	if err != nil {
		return "", err
	}
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12], nil
}

func generateOpenSSHEd25519KeyPair(ctx context.Context) (privateKey string, publicKey string, err error) {
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	default:
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", err
	}
	privateKey = strings.TrimSpace(string(pem.EncodeToMemory(privBlock)))
	pubKey, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return "", "", err
	}
	publicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pubKey)))
	if !strings.Contains(privateKey, "BEGIN OPENSSH PRIVATE KEY") || !strings.Contains(privateKey, "END OPENSSH PRIVATE KEY") {
		return "", "", fmt.Errorf("生成私钥格式异常")
	}
	if !strings.HasPrefix(publicKey, "ssh-ed25519 ") {
		return "", "", fmt.Errorf("生成公钥格式异常")
	}
	return privateKey, publicKey, nil
}

func inferSSHPort(nodeID string, reqPort int) int {
	if reqPort > 0 && reqPort <= 65535 {
		return reqPort
	}
	if n, err := strconv.Atoi(strings.TrimSpace(nodeID)); err == nil && n > 0 && n <= 65535 {
		return n
	}
	return 22
}

func inferRequestBaseURL(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return ""
	}
	return strings.TrimRight(fmt.Sprintf("%s://%s", proto, host), "/")
}
