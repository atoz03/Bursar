package main

import (
	"testing"
	"time"
)

func TestGenerateTOTPCodeRFCVectors(t *testing.T) {
	secret := totpBase32Encoding.EncodeToString([]byte("12345678901234567890"))
	cases := []struct {
		unix int64
		want string
	}{
		{unix: 59, want: "287082"},
		{unix: 1111111109, want: "081804"},
		{unix: 1234567890, want: "005924"},
	}
	for _, tc := range cases {
		got, err := generateTOTPCode(secret, time.Unix(tc.unix, 0))
		if err != nil {
			t.Fatalf("generateTOTPCode(%d) error: %v", tc.unix, err)
		}
		if got != tc.want {
			t.Fatalf("generateTOTPCode(%d) = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

func TestVerifyTOTPCodeAllowsOneStepSkew(t *testing.T) {
	secret, err := generateTOTPSecret()
	if err != nil {
		t.Fatalf("generateTOTPSecret error: %v", err)
	}
	now := time.Unix(1710000000, 0)
	code, err := generateTOTPCode(secret, now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("generateTOTPCode error: %v", err)
	}
	if !verifyTOTPCode(secret, code, now) {
		t.Fatalf("verifyTOTPCode should accept one-step skew")
	}
	if verifyTOTPCode(secret, "000000", now) {
		t.Fatalf("verifyTOTPCode should reject wrong code")
	}
}
