package main

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	captchaAddPattern = regexp.MustCompile(`^简单验证：(\d+) \+ (\d+) = \?$`)
	captchaSubPattern = regexp.MustCompile(`^简单验证：(\d+) - (\d+) = \?$`)
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
	if m := captchaAddPattern.FindStringSubmatch(question); len(m) == 3 {
		return "add", mustAtoiForTest(t, m[1]) + mustAtoiForTest(t, m[2])
	}
	if m := captchaSubPattern.FindStringSubmatch(question); len(m) == 3 {
		return "sub", mustAtoiForTest(t, m[1]) - mustAtoiForTest(t, m[2])
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
	if len(seenKinds) != 2 {
		t.Fatalf("seen captcha kinds=%v, want add and sub", seenKinds)
	}
}

func TestNormalizeRegisterEmail(t *testing.T) {
	t.Parallel()

	normalized, local, domain, err := normalizeRegisterEmail("user@Example.org", []string{"example.org"})
	if err != nil {
		t.Fatalf("normalizeRegisterEmail() error: %v", err)
	}
	if normalized != "USER@example.org" {
		t.Fatalf("normalized=%q", normalized)
	}
	if local != "USER" {
		t.Fatalf("local=%q", local)
	}
	if domain != "example.org" {
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
