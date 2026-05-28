package main

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminDisposableDomainUpsertReq struct {
	Domain  string `json:"domain"`
	Enabled *bool  `json:"enabled"`
	Note    string `json:"note"`
}

func (s *Server) issueAuthCaptcha(c *gin.Context) {
	captchaID, err := randomTokenURLSafe(24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	question, options, answerIndex, err := buildNumericChoiceCaptcha()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	expireAt := time.Now().Add(registerCaptchaTTL)
	clientIP := trimmedClientIP(c.ClientIP())
	if err := s.store.CreateRegisterCaptchaChallenge(
		c.Request.Context(),
		captchaID,
		clientIP,
		question,
		options,
		answerIndex,
		expireAt,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"captcha_id": captchaID,
		"question":   question,
		"options":    options,
		"expire_at":  formatRFC3339InBeijing(expireAt),
	})
}

func (s *Server) handleAuthLoginCaptcha(c *gin.Context) {
	s.issueAuthCaptcha(c)
}

func (s *Server) handleAuthRegisterCaptcha(c *gin.Context) {
	s.issueAuthCaptcha(c)
}

func (s *Server) handleAdminRegisterSecurityPolicy(c *gin.Context) {
	domains := make([]string, 0, len(allowedRegisterEmailDomains))
	for d := range allowedRegisterEmailDomains {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	c.JSON(http.StatusOK, gin.H{
		"ip_window_seconds":        int(registerIPWindow / time.Second),
		"ip_limit":                 registerIPLimit,
		"email_window_seconds":     int(registerEmailWindow / time.Second),
		"email_limit":              registerEmailLimit,
		"ip_cooldown_seconds":      int(registerIPCooldown / time.Second),
		"ip_cooldown_failures":     registerIPCooldownFailures,
		"email_cooldown_seconds":   int(registerEmailCooldown / time.Second),
		"captcha_ttl_seconds":      int(registerCaptchaTTL / time.Second),
		"captcha_option_count":     registerCaptchaOptionCount,
		"allowed_email_domains":    domains,
		"allowed_email_suffix_tip": allowedRegisterEmailSuffixHint(),
	})
}

func (s *Server) handleAdminRegisterSecurityEvents(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	field := strings.TrimSpace(c.Query("field"))
	action := strings.TrimSpace(c.Query("action"))
	decision := strings.TrimSpace(c.Query("decision"))
	limit := parseLimit(c.Query("limit"), registerSecurityDefaultLimit, 5000)
	rows, err := s.store.ListRegistrationSecurityEvents(c.Request.Context(), keyword, field, action, decision, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": rows})
}

func (s *Server) handleAdminDisposableEmailDomainsList(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	limit := parseLimit(c.Query("limit"), 1000, 5000)
	rows, err := s.store.ListDisposableEmailDomains(c.Request.Context(), keyword, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"domains": rows})
}

func (s *Server) handleAdminDisposableEmailDomainsUpsert(c *gin.Context) {
	var req adminDisposableDomainUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := s.store.UpsertDisposableEmailDomain(
		c.Request.Context(),
		req.Domain,
		enabled,
		req.Note,
		s.currentOperator(c),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "domain": row})
}

func (s *Server) handleAdminDisposableEmailDomainsDelete(c *gin.Context) {
	domain := strings.TrimSpace(c.Param("domain"))
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain 不能为空"})
		return
	}
	if err := s.store.DeleteDisposableEmailDomain(c.Request.Context(), domain); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "域名不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
