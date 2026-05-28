package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	captchaAddPattern      = regexp.MustCompile(`^请计算：(\d+) \+ (\d+) \+ (\d+) = \?$`)
	captchaMulSubPattern   = regexp.MustCompile(`^请计算：(\d+) × (\d+) - (\d+) = \?$`)
	captchaCountPattern    = regexp.MustCompile(`^观察序列：([0-9 ]+)。数字 (\d) 一共出现了几次？$`)
	captchaGCDPattern      = regexp.MustCompile(`^请计算 (\d+) 和 (\d+) 的最大公约数。$`)
	captchaDigitSumPattern = regexp.MustCompile(`^请计算数字 (\d+) 的各位数字之和。$`)
)

func mustAtoiForTest(t *testing.T, raw string) int {
	t.Helper()
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("strconv.Atoi(%q) failed: %v", raw, err)
	}
	return v
}

func solveCaptchaQuestionForTest(t *testing.T, question string) (string, int) {
	t.Helper()
	if m := captchaAddPattern.FindStringSubmatch(question); len(m) == 4 {
		return "add", mustAtoiForTest(t, m[1]) + mustAtoiForTest(t, m[2]) + mustAtoiForTest(t, m[3])
	}
	if m := captchaMulSubPattern.FindStringSubmatch(question); len(m) == 4 {
		return "mul_sub", mustAtoiForTest(t, m[1])*mustAtoiForTest(t, m[2]) - mustAtoiForTest(t, m[3])
	}
	if m := captchaCountPattern.FindStringSubmatch(question); len(m) == 3 {
		target := mustAtoiForTest(t, m[2])
		count := 0
		for _, part := range strings.Fields(m[1]) {
			if mustAtoiForTest(t, part) == target {
				count++
			}
		}
		return "count", count
	}
	if m := captchaGCDPattern.FindStringSubmatch(question); len(m) == 3 {
		return "gcd", gcdInt(mustAtoiForTest(t, m[1]), mustAtoiForTest(t, m[2]))
	}
	if m := captchaDigitSumPattern.FindStringSubmatch(question); len(m) == 2 {
		return "digit_sum", sumDigitsInt(mustAtoiForTest(t, m[1]))
	}
	t.Fatalf("unexpected captcha question: %q", question)
	return "", 0
}

func TestBuildNumericChoiceCaptcha(t *testing.T) {
	t.Parallel()

	seenKinds := map[string]bool{}
	for i := 0; i < 400; i++ {
		question, options, answerIndex, err := buildNumericChoiceCaptcha()
		if err != nil {
			t.Fatalf("buildNumericChoiceCaptcha() error: %v", err)
		}
		if strings.TrimSpace(question) == "" {
			t.Fatalf("empty question on iteration %d", i)
		}
		if len(options) != registerCaptchaOptionCount {
			t.Fatalf("len(options)=%d, want %d", len(options), registerCaptchaOptionCount)
		}
		if answerIndex < 0 || answerIndex >= len(options) {
			t.Fatalf("invalid answerIndex=%d for %d options", answerIndex, len(options))
		}
		unique := make(map[int]struct{}, len(options))
		for _, option := range options {
			if _, ok := unique[option]; ok {
				t.Fatalf("duplicate option %d in %v", option, options)
			}
			unique[option] = struct{}{}
		}

		kind, expected := solveCaptchaQuestionForTest(t, question)
		seenKinds[kind] = true
		if got := options[answerIndex]; got != expected {
			t.Fatalf("question=%q answer option=%d, want %d (options=%v index=%d)", question, got, expected, options, answerIndex)
		}
	}
	if len(seenKinds) != 5 {
		t.Fatalf("seen captcha kinds=%v, want all 5 kinds", seenKinds)
	}
}

func TestShouldTriggerRegisterIPCooldown(t *testing.T) {
	t.Parallel()

	now := time.Now()
	recent := now.Add(-(registerIPCooldown - time.Second))
	expired := now.Add(-(registerIPCooldown + time.Second))

	if shouldTriggerRegisterIPCooldown(RegistrationRateStats{}, now) {
		t.Fatal("unexpected cooldown for empty stats")
	}
	if shouldTriggerRegisterIPCooldown(RegistrationRateStats{
		RecentIPFailureCount: registerIPCooldownFailures - 1,
		LastIPFailureAt:      &recent,
	}, now) {
		t.Fatal("unexpected cooldown below failure threshold")
	}
	if shouldTriggerRegisterIPCooldown(RegistrationRateStats{
		RecentIPFailureCount: registerIPCooldownFailures,
		LastIPFailureAt:      &expired,
	}, now) {
		t.Fatal("unexpected cooldown after cooldown window expired")
	}
	if !shouldTriggerRegisterIPCooldown(RegistrationRateStats{
		RecentIPFailureCount: registerIPCooldownFailures,
		LastIPFailureAt:      &recent,
	}, now) {
		t.Fatal("expected cooldown for recent failures at threshold")
	}
}

func TestNormalizeRegisterEmail(t *testing.T) {
	t.Parallel()

	normalized, local, domain, err := normalizeRegisterEmail("26b123456@Stu.HIT.edu.cn")
	if err != nil {
		t.Fatalf("normalizeRegisterEmail() error: %v", err)
	}
	if normalized != "26B123456@students.example.org" {
		t.Fatalf("normalized=%q", normalized)
	}
	if local != "26B123456" {
		t.Fatalf("local=%q", local)
	}
	if domain != "students.example.org" {
		t.Fatalf("domain=%q", domain)
	}
}

func TestValidateRegisterUsername(t *testing.T) {
	t.Parallel()

	if err := validateRegisterUsername("zs26B123456", "26B123456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateRegisterUsername("zs26B123456", ""); err == nil {
		t.Fatal("expected error for empty email local")
	}
	if err := validateRegisterUsername("z126B123456", "26B123456"); err == nil {
		t.Fatal("expected error for invalid prefix")
	}
}
