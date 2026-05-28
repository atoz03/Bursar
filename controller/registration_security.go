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
	registerCaptchaTTL           = 2 * time.Minute
	registerCaptchaOptionCount   = 10
	registerIPWindow             = 10 * time.Minute
	registerIPLimit              = 25
	registerEmailWindow          = 30 * time.Minute
	registerEmailLimit           = 3
	registerIPCooldown           = 300 * time.Second
	registerIPCooldownFailures   = 2
	registerEmailCooldown        = 90 * time.Second
	registerSecurityDefaultLimit = 500
	registerUsernamePrefixMinLen = 2
	registerUsernamePrefixMaxLen = 8
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

func normalizeRegisterEmail(rawEmail string) (string, string, string, error) {
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
	if _, ok := allowedRegisterEmailDomains[domain]; !ok {
		return "", "", "", fmt.Errorf("注册邮箱后缀仅支持 %s", allowedRegisterEmailSuffixHint())
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

func shouldTriggerRegisterIPCooldown(stats RegistrationRateStats, now time.Time) bool {
	if stats.RecentIPFailureCount < registerIPCooldownFailures {
		return false
	}
	if stats.LastIPFailureAt == nil {
		return false
	}
	return now.Sub(*stats.LastIPFailureAt) < registerIPCooldown
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

func gcdInt(a int, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func sumDigitsInt(v int) int {
	if v < 0 {
		v = -v
	}
	sum := 0
	for v > 0 {
		sum += v % 10
		v /= 10
	}
	return sum
}

func buildCaptchaOptions(answer int, minValue int) ([]int, int, error) {
	candidates := []int{-41, -29, -23, -19, -17, -13, -11, -9, -7, -5, -3, -2, 2, 3, 5, 7, 9, 11, 13, 17, 19, 23, 29, 41}
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
	kind, err := randomInt(0, 4)
	if err != nil {
		return "", nil, 0, err
	}

	question := ""
	answer := 0
	minValue := 0
	switch kind {
	case 0:
		a, err := randomInt(12, 79)
		if err != nil {
			return "", nil, 0, err
		}
		b, err := randomInt(8, 43)
		if err != nil {
			return "", nil, 0, err
		}
		c, err := randomInt(3, 21)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("请计算：%d + %d + %d = ?", a, b, c)
		answer = a + b + c
	case 1:
		a, err := randomInt(14, 48)
		if err != nil {
			return "", nil, 0, err
		}
		b, err := randomInt(2, 8)
		if err != nil {
			return "", nil, 0, err
		}
		c, err := randomInt(5, a*b-1)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("请计算：%d × %d - %d = ?", a, b, c)
		answer = a*b - c
	case 2:
		digit, err := randomInt(1, 9)
		if err != nil {
			return "", nil, 0, err
		}
		length, err := randomInt(10, 14)
		if err != nil {
			return "", nil, 0, err
		}
		parts := make([]string, 0, length)
		count := 0
		for i := 0; i < length; i++ {
			v, err := randomInt(0, 9)
			if err != nil {
				return "", nil, 0, err
			}
			if v == digit {
				count++
			}
			parts = append(parts, fmt.Sprintf("%d", v))
		}
		question = fmt.Sprintf("观察序列：%s。数字 %d 一共出现了几次？", strings.Join(parts, " "), digit)
		answer = count
	case 3:
		base, err := randomInt(6, 36)
		if err != nil {
			return "", nil, 0, err
		}
		pairs := [][2]int{{8, 3}, {9, 4}, {7, 5}, {11, 2}, {13, 4}, {5, 6}}
		pairIdx, err := randomInt(0, len(pairs)-1)
		if err != nil {
			return "", nil, 0, err
		}
		leftFactor := pairs[pairIdx][0]
		rightFactor := pairs[pairIdx][1]
		left := base * leftFactor
		right := base * rightFactor
		question = fmt.Sprintf("请计算 %d 和 %d 的最大公约数。", left, right)
		answer = gcdInt(left, right)
	default:
		n, err := randomInt(1000, 99999)
		if err != nil {
			return "", nil, 0, err
		}
		question = fmt.Sprintf("请计算数字 %d 的各位数字之和。", n)
		answer = sumDigitsInt(n)
	}
	options, answerIndex, err := buildCaptchaOptions(answer, minValue)
	if err != nil {
		return "", nil, 0, err
	}
	return question, options, answerIndex, nil
}
