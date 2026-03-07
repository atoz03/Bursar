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
)

const (
	registerCaptchaTTL           = 2 * time.Minute
	registerCaptchaOptionCount   = 10
	registerIPWindow             = 10 * time.Minute
	registerIPLimit              = 25
	registerEmailWindow          = 30 * time.Minute
	registerEmailLimit           = 3
	registerIPCooldown           = 8 * time.Second
	registerEmailCooldown        = 90 * time.Second
	registerSecurityDefaultLimit = 500
)

var (
	allowedRegisterEmailDomains = map[string]struct{}{
		"example.org":     {},
		"students.example.org": {},
	}
)

func allowedRegisterEmailSuffixHint() string {
	return "@example.org 或 @students.example.org"
}

func normalizeStudentID(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func normalizeRegisterEmail(rawEmail string, studentIDUpper string) (string, string, error) {
	email := strings.TrimSpace(rawEmail)
	if email == "" {
		return "", "", errors.New("邮箱不能为空")
	}
	if strings.ContainsAny(email, " \t\r\n") || strings.Count(email, "@") != 1 {
		return "", "", errors.New("邮箱格式不合法")
	}
	parts := strings.SplitN(email, "@", 2)
	local := strings.ToUpper(strings.TrimSpace(parts[0]))
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	if local == "" || domain == "" {
		return "", "", errors.New("邮箱格式不合法")
	}
	if _, ok := allowedRegisterEmailDomains[domain]; !ok {
		return "", "", fmt.Errorf("注册邮箱后缀仅支持 %s", allowedRegisterEmailSuffixHint())
	}
	if studentIDUpper == "" {
		return "", "", errors.New("学号不能为空")
	}
	if local != studentIDUpper {
		return "", "", errors.New("邮箱前缀必须与学号一致（邮箱前缀=学号）")
	}
	normalized := local + "@" + domain
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", "", errors.New("邮箱格式不合法")
	}
	return normalized, domain, nil
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

func buildRegisterMathCaptcha() (string, []int, int, error) {
	kind, err := randomInt(0, 3)
	if err != nil {
		return "", nil, 0, err
	}
	a, err := randomInt(12, 97)
	if err != nil {
		return "", nil, 0, err
	}
	b, err := randomInt(3, 27)
	if err != nil {
		return "", nil, 0, err
	}
	c, err := randomInt(2, 9)
	if err != nil {
		return "", nil, 0, err
	}

	question := ""
	answer := 0
	switch kind {
	case 0:
		question = fmt.Sprintf("请计算：(%d + %d) × %d = ?", a, b, c)
		answer = (a + b) * c
	case 1:
		question = fmt.Sprintf("请计算：%d + %d × %d = ?", a, b, c)
		answer = a + b*c
	case 2:
		question = fmt.Sprintf("请计算：(%d - %d) × %d = ?", a, b, c)
		answer = (a - b) * c
	default:
		d, err := randomInt(2, 8)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("请计算：(%d + %d) ÷ %d = ?", a*d, b*d, d)
		answer = (a*d + b*d) / d
	}

	candidates := []int{-17, -13, -9, -7, -5, -3, -2, 2, 3, 5, 7, 9, 11, 13, 17}
	choices := map[int]struct{}{
		answer: {},
	}
	for len(choices) < registerCaptchaOptionCount {
		idx, err := randomInt(0, len(candidates)-1)
		if err != nil {
			return "", nil, 0, err
		}
		v := answer + candidates[idx]
		if v == answer {
			continue
		}
		choices[v] = struct{}{}
	}
	options := make([]int, 0, len(choices))
	for v := range choices {
		options = append(options, v)
	}
	if err := shuffleInts(options); err != nil {
		return "", nil, 0, err
	}
	answerIndex := -1
	for i, v := range options {
		if v == answer {
			answerIndex = i
			break
		}
	}
	if answerIndex < 0 {
		return "", nil, 0, errors.New("captcha generation failed")
	}
	return question, options, answerIndex, nil
}
