package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	registerCaptchaTTL           = 10 * time.Minute
	registerCaptchaOptionCount   = 4
	registerIPWindow             = 10 * time.Minute
	registerIPLimit              = 25
	registerEmailWindow          = 30 * time.Minute
	registerEmailLimit           = 3
	registerEmailCooldown        = 90 * time.Second
	legacyRegisterIPCooldown     = 300 * time.Second
	registerSecurityDefaultLimit = 500
	registerUsernamePrefixMinLen = 2
	registerUsernamePrefixMaxLen = 8
)

func allowedRegisterEmailSuffixHint(domains []string) string {
	if len(domains) == 0 {
		return "任意有效邮箱域名"
	}
	parts := make([]string, 0, len(domains))
	for _, domain := range domains {
		parts = append(parts, "@"+domain)
	}
	return strings.Join(parts, "、")
}

func normalizeStudentID(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func normalizeRegisterEmail(rawEmail string, allowedDomains []string) (string, string, string, error) {
	email := strings.TrimSpace(rawEmail)
	if email == "" {
		return "", "", "", errors.New("邮箱不能为空")
	}
	if strings.ContainsAny(email, " \t\r\n") || strings.Count(email, "@") != 1 {
		return "", "", "", errors.New("邮箱格式不合法")
	}
	parts := strings.SplitN(email, "@", 2)
	local := strings.ToUpper(strings.TrimSpace(parts[0]))
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	if local == "" || domain == "" {
		return "", "", "", errors.New("邮箱格式不合法")
	}
	if len(allowedDomains) > 0 {
		allowed := false
		for _, allowedDomain := range allowedDomains {
			if domain == strings.ToLower(strings.TrimSpace(allowedDomain)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", "", "", fmt.Errorf("注册邮箱后缀仅支持 %s", allowedRegisterEmailSuffixHint(allowedDomains))
		}
	}
	normalized := local + "@" + domain
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", "", "", errors.New("邮箱格式不合法")
	}
	return normalized, local, domain, nil
}

func validateRegisterUsername(username string, emailLocal string) error {
	username = strings.TrimSpace(username)
	emailLocal = strings.ToUpper(strings.TrimSpace(emailLocal))
	if username == "" {
		return errors.New("用户名不能为空")
	}
	if emailLocal == "" {
		return errors.New("请先填写合法邮箱，再按“姓名缩写+邮箱前缀”格式填写用户名")
	}
	if utf8.RuneCountInString(username) > 18 {
		return errors.New("用户名不得超过 18 个字符")
	}
	if !strings.HasSuffix(username, emailLocal) {
		return fmt.Errorf("用户名必须以邮箱前缀 %s 结尾，例如 zs%s", emailLocal, emailLocal)
	}
	prefix := strings.TrimSuffix(username, emailLocal)
	if len(prefix) < registerUsernamePrefixMinLen || len(prefix) > registerUsernamePrefixMaxLen {
		return fmt.Errorf("用户名必须写成“姓名缩写+邮箱前缀”，前缀需为 %d-%d 个小写字母，例如 zs%s", registerUsernamePrefixMinLen, registerUsernamePrefixMaxLen, emailLocal)
	}
	for _, r := range prefix {
		if r < 'a' || r > 'z' {
			return fmt.Errorf("用户名必须写成“姓名缩写+邮箱前缀”，前缀只能使用小写英文字母，例如 zs%s", emailLocal)
		}
	}
	return nil
}

func trimmedClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "unknown"
	}
	if len(ip) > 64 {
		return ip[:64]
	}
	return ip
}

func trimmedUserAgent(ua string) string {
	ua = strings.TrimSpace(ua)
	if len(ua) > 512 {
		return ua[:512]
	}
	return ua
}

func randomInt(min, max int) (int, error) {
	if min > max {
		return 0, errors.New("invalid random range")
	}
	n := max - min + 1
	if n <= 0 {
		return 0, errors.New("invalid random range")
	}
	x, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return min + int(x.Int64()), nil
}

func randomTokenURLSafe(bytesLen int) (string, error) {
	if bytesLen <= 0 {
		bytesLen = 24
	}
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "="), nil
}

func shuffleInts(values []int) error {
	for i := len(values) - 1; i > 0; i-- {
		j, err := randomInt(0, i)
		if err != nil {
			return err
		}
		values[i], values[j] = values[j], values[i]
	}
	return nil
}

func buildCaptchaOptions(answer int, minValue int) ([]int, int, error) {
	candidates := []int{-4, -3, -2, -1, 1, 2, 3, 4}
	choices := map[int]struct{}{
		answer: {},
	}
	for len(choices) < registerCaptchaOptionCount {
		idx, err := randomInt(0, len(candidates)-1)
		if err != nil {
			return nil, 0, err
		}
		v := answer + candidates[idx]
		if v < minValue {
			continue
		}
		choices[v] = struct{}{}
	}
	options := make([]int, 0, len(choices))
	for v := range choices {
		options = append(options, v)
	}
	if err := shuffleInts(options); err != nil {
		return nil, 0, err
	}
	answerIndex := -1
	for i, v := range options {
		if v == answer {
			answerIndex = i
			break
		}
	}
	if answerIndex < 0 {
		return nil, 0, errors.New("captcha generation failed")
	}
	return options, answerIndex, nil
}

func buildNumericChoiceCaptcha() (string, []int, int, error) {
	kind, err := randomInt(0, 1)
	if err != nil {
		return "", nil, 0, err
	}

	question := ""
	answer := 0
	minValue := 0
	switch kind {
	case 0:
		a, err := randomInt(2, 15)
		if err != nil {
			return "", nil, 0, err
		}
		b, err := randomInt(1, 12)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("简单验证：%d + %d = ?", a, b)
		answer = a + b
	default:
		a, err := randomInt(8, 20)
		if err != nil {
			return "", nil, 0, err
		}
		b, err := randomInt(1, a-1)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("简单验证：%d - %d = ?", a, b)
		answer = a - b
	}
	options, answerIndex, err := buildCaptchaOptions(answer, minValue)
	if err != nil {
		return "", nil, 0, err
	}
	return question, options, answerIndex, nil
}
