package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpIssuer = "GPU Ops"
	totpDigits = 6
	totpPeriod = 30
)

var totpBase32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func generateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return totpBase32Encoding.EncodeToString(buf), nil
}

func normalizeTOTPCode(code string) string {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	return code
}

func generateTOTPCode(secret string, now time.Time) (string, error) {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	if secret == "" {
		return "", fmt.Errorf("totp secret 不能为空")
	}
	key, err := totpBase32Encoding.DecodeString(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(now.Unix() / totpPeriod)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 0x0f)
	truncated := binary.BigEndian.Uint32(sum[offset : offset+4])
	truncated &= 0x7fffffff
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, truncated%mod), nil
}

func verifyTOTPCode(secret string, code string, now time.Time) bool {
	code = normalizeTOTPCode(code)
	if len(code) != totpDigits {
		return false
	}
	for _, skew := range []int{-1, 0, 1} {
		candidate, err := generateTOTPCode(secret, now.Add(time.Duration(skew*totpPeriod)*time.Second))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func totpAccountName(role string, username string) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(role), strings.TrimSpace(username))
}

func buildTOTPKeyURI(role string, username string, secret string) string {
	label := url.PathEscape(totpIssuer) + ":" + url.PathEscape(totpAccountName(role, username))
	q := url.Values{}
	q.Set("secret", strings.TrimSpace(secret))
	q.Set("issuer", totpIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
