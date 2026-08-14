package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 这些用例覆盖「余额/用量/指标接口曾经完全公开」这个缺陷的修复。
// 它们不依赖数据库：nil store 的分支在解析会话之前就已返回。

func newAuthTestServer() *Server {
	return NewServer(Config{
		AdminToken:   "admin-token-value",
		AgentToken:   "agent-token-value",
		SessionHours: 12,
		AuthSecret:   "0123456789abcdef0123456789abcdef",
	}, nil)
}

func doAuthRequest(t *testing.T, mw gin.HandlerFunc, path string, headers map[string]string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:username/balance", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/metrics", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestAuthOperatorRejectsAnonymous(t *testing.T) {
	s := newAuthTestServer()
	if got := doAuthRequest(t, s.authOperator(), "/metrics", nil); got != http.StatusUnauthorized {
		t.Fatalf("匿名访问 /metrics 应返回 401，实际 %d", got)
	}
}

func TestAuthOperatorRejectsWrongToken(t *testing.T) {
	s := newAuthTestServer()
	cases := []map[string]string{
		{"Authorization": "Bearer wrong-token"},
		{"Authorization": "admin-token-value"}, // 缺少 Bearer 前缀
		{"X-Agent-Token": "wrong-token"},
	}
	for i, h := range cases {
		if got := doAuthRequest(t, s.authOperator(), "/metrics", h); got != http.StatusUnauthorized {
			t.Fatalf("用例 %d 应返回 401，实际 %d", i, got)
		}
	}
}

func TestAuthOperatorAcceptsOperatorTokens(t *testing.T) {
	s := newAuthTestServer()
	cases := []map[string]string{
		{"Authorization": "Bearer admin-token-value"},
		{"X-Agent-Token": "agent-token-value"},
	}
	for i, h := range cases {
		if got := doAuthRequest(t, s.authOperator(), "/metrics", h); got != http.StatusOK {
			t.Fatalf("用例 %d 应放行，实际 %d", i, got)
		}
	}
}

func TestAuthSelfOrOperatorRejectsAnonymousBalance(t *testing.T) {
	s := newAuthTestServer()
	if got := doAuthRequest(t, s.authSelfOrOperator(), "/users/alice/balance", nil); got != http.StatusUnauthorized {
		t.Fatalf("匿名查询余额应返回 401，实际 %d", got)
	}
}

func TestAuthSelfOrOperatorAcceptsOperatorTokens(t *testing.T) {
	s := newAuthTestServer()
	cases := []map[string]string{
		{"Authorization": "Bearer admin-token-value"},
		{"X-Agent-Token": "agent-token-value"},
	}
	for i, h := range cases {
		if got := doAuthRequest(t, s.authSelfOrOperator(), "/users/alice/balance", h); got != http.StatusOK {
			t.Fatalf("用例 %d 应放行，实际 %d", i, got)
		}
	}
}

func TestAuthOperatorRejectsWhenSessionsDisabled(t *testing.T) {
	// session_hours=0 时不存在会话通道，仍必须拒绝匿名请求而不是触碰 nil store。
	s := NewServer(Config{AdminToken: "admin-token-value", AgentToken: "agent-token-value"}, nil)
	if got := doAuthRequest(t, s.authOperator(), "/metrics", nil); got != http.StatusUnauthorized {
		t.Fatalf("禁用会话时匿名访问应返回 401，实际 %d", got)
	}
}

// TestRouterWebProtectsReadOnlyRoutes 走真实路由表，确认中间件确实挂在了这三个接口上，
// 而不只是在单元测试里被单独调用。
func TestRouterWebProtectsReadOnlyRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newAuthTestServer()
	r := s.RouterWeb()

	for _, path := range []string{"/metrics", "/api/users/alice/balance", "/api/users/alice/usage"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("匿名访问 %s 应返回 401，实际 %d", path, w.Code)
		}
	}

	// /healthz 必须保持公开，否则负载均衡探活会失败。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/healthz 应保持公开，实际 %d", w.Code)
	}

	// 携带 admin_token 时 /metrics 必须可用，否则 Prometheus 抓取会中断。
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer admin-token-value")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("带 admin_token 抓取 /metrics 应返回 200，实际 %d", w.Code)
	}
}

func TestSelfOrOperatorAllowed(t *testing.T) {
	cases := []struct {
		name    string
		role    string
		session string
		path    string
		want    bool
	}{
		{"管理员查询他人", "admin", "root", "alice", true},
		{"高级用户查询他人", "power_user", "ops", "alice", true},
		{"普通用户查询本人", "user", "alice", "alice", true},
		{"普通用户查询本人-大小写不同", "user", "Alice", "alice", true},
		{"普通用户查询本人-两端空格", "user", " alice ", "alice", true},
		{"普通用户查询他人", "user", "alice", "bob", false},
		{"会话用户名为空", "user", "", "", false},
		{"路径用户名为空", "user", "alice", "", false},
		{"未知角色查询他人", "", "alice", "bob", false},
	}
	for _, tc := range cases {
		if got := selfOrOperatorAllowed(tc.role, tc.session, tc.path); got != tc.want {
			t.Fatalf("%s：期望 %v，实际 %v", tc.name, tc.want, got)
		}
	}
}
