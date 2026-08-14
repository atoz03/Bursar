package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errAuthCaptchaUnavailable = errors.New("auth captcha service unavailable")

type turnstileSiteverifyResponse struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	ErrorCodes []string `json:"error-codes"`
}

func (s *Server) turnstileEnabled() bool {
	return strings.TrimSpace(s.cfg.TurnstileSiteKey) != "" && strings.TrimSpace(s.cfg.TurnstileSecretKey) != ""
}

func (s *Server) verifyTurnstile(ctx context.Context, token string, expectedAction string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errRegisterCaptchaInvalid
	}
	form := url.Values{}
	form.Set("secret", strings.TrimSpace(s.cfg.TurnstileSecretKey))
	form.Set("response", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.turnstileVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", errAuthCaptchaUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.turnstileHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errAuthCaptchaUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: siteverify status %d", errAuthCaptchaUnavailable, resp.StatusCode)
	}
	var result turnstileSiteverifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("%w: %v", errAuthCaptchaUnavailable, err)
	}
	if !result.Success {
		return fmt.Errorf("%w: %s", errRegisterCaptchaInvalid, strings.Join(result.ErrorCodes, ","))
	}
	if strings.TrimSpace(expectedAction) != "" && strings.TrimSpace(result.Action) != strings.TrimSpace(expectedAction) {
		return fmt.Errorf("%w: action mismatch", errRegisterCaptchaInvalid)
	}
	hostname := strings.ToLower(strings.TrimSpace(result.Hostname))
	for _, expected := range s.cfg.TurnstileExpectedHostnames {
		if hostname == strings.ToLower(strings.TrimSpace(expected)) {
			return nil
		}
	}
	return fmt.Errorf("%w: hostname mismatch", errRegisterCaptchaInvalid)
}

func (s *Server) consumeLocalLoginCaptcha(ctx context.Context, captchaID string, now time.Time) error {
	if s.turnstileEnabled() {
		return nil
	}
	return s.store.ConsumeRegisterCaptcha(ctx, captchaID, now)
}
