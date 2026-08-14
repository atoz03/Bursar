package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTurnstileTestServer(t *testing.T, response string, status int) (*Server, func()) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if r.Form.Get("secret") != "test-secret" {
			t.Fatalf("secret=%q", r.Form.Get("secret"))
		}
		if r.Form.Get("response") != "test-token" {
			t.Fatalf("response=%q", r.Form.Get("response"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	s := NewServer(Config{
		TurnstileSiteKey:           "test-site-key",
		TurnstileSecretKey:         "test-secret",
		TurnstileExpectedHostnames: []string{"192.0.2.20"},
	}, nil)
	s.turnstileVerifyURL = upstream.URL
	s.turnstileHTTPClient = upstream.Client()
	return s, upstream.Close
}

func TestVerifyTurnstile(t *testing.T) {
	t.Run("accepts expected action and internal hostname", func(t *testing.T) {
		s, closeServer := newTurnstileTestServer(t, `{"success":true,"hostname":"192.0.2.20","action":"login"}`, http.StatusOK)
		defer closeServer()
		if err := s.verifyTurnstile(context.Background(), "test-token", "login"); err != nil {
			t.Fatalf("verifyTurnstile() error: %v", err)
		}
	})

	t.Run("rejects a token from another hostname", func(t *testing.T) {
		s, closeServer := newTurnstileTestServer(t, `{"success":true,"hostname":"example.com","action":"login"}`, http.StatusOK)
		defer closeServer()
		err := s.verifyTurnstile(context.Background(), "test-token", "login")
		if !errors.Is(err, errRegisterCaptchaInvalid) {
			t.Fatalf("error=%v, want captcha invalid", err)
		}
	})

	t.Run("reports upstream failures as unavailable", func(t *testing.T) {
		s, closeServer := newTurnstileTestServer(t, `{}`, http.StatusBadGateway)
		defer closeServer()
		err := s.verifyTurnstile(context.Background(), "test-token", "login")
		if !errors.Is(err, errAuthCaptchaUnavailable) {
			t.Fatalf("error=%v, want captcha unavailable", err)
		}
	})
}
