package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Server struct {
	cfg   Config
	store *Store
	queue *Queue
	metr  *controllerMetrics

	nodeActionsMu sync.Mutex
	nodeActions   map[string][]Action
}

const (
	permViewBoard      = 1
	permViewNodes      = 2
	permReviewRequests = 4
)

func NewServer(cfg Config, store *Store) *Server {
	return &Server{
		cfg:         cfg,
		store:       store,
		queue:       NewQueue(),
		metr:        &controllerMetrics{},
		nodeActions: make(map[string][]Action),
	}
}

func (s *Server) enqueueNodeAction(nodeID string, action Action) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return
	}
	s.nodeActionsMu.Lock()
	defer s.nodeActionsMu.Unlock()
	s.nodeActions[nodeID] = append(s.nodeActions[nodeID], action)
}

func (s *Server) popNodeActions(nodeID string) []Action {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	s.nodeActionsMu.Lock()
	defer s.nodeActionsMu.Unlock()
	out := s.nodeActions[nodeID]
	delete(s.nodeActions, nodeID)
	return out
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/metrics", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, s.metr.render(s.queue.Len()))
	})

	api := r.Group("/api")
	api.GET("/auth/me", s.handleAuthMe)
	api.POST("/auth/login", s.handleAuthLogin)
	api.POST("/auth/logout", s.handleAuthLogout)
	api.POST("/auth/register", s.handleAuthRegister)
	api.POST("/auth/forgot-password", s.handleAuthForgotPassword)
	api.POST("/auth/reset-password", s.handleAuthResetPassword)
	api.POST("/auth/change-password", s.authSession(), s.handleAuthChangePassword)
	api.GET("/announcements", s.handleAnnouncementsList)

	api.POST("/metrics", s.authAgent(), s.handleMetrics)
	api.GET("/node/actions", s.authAgent(), s.handleNodeActions)

	api.GET("/users/:username/balance", s.handleBalance)
	api.GET("/users/:username/usage", s.handleUserUsage)
	api.POST("/users/:username/recharge", s.authAdmin(), s.requireSuperAdmin(), s.handleRecharge)

	user := api.Group("/user")
	user.Use(s.authSession())
	user.GET("/me", s.handleUserMe)
	user.GET("/me/balance", s.handleUserMyBalance)
	user.GET("/me/usage", s.handleUserMyUsage)
	user.PUT("/me/profile", s.handleUserMyProfileUpdate)
	user.GET("/me/profile-change-requests", s.handleUserMyProfileChangeRequests)
	user.GET("/accounts", s.handleUserAccountsList)
	user.POST("/accounts", s.handleUserAccountsUpsert)
	user.PUT("/accounts", s.handleUserAccountsUpdate)
	user.DELETE("/accounts", s.handleUserAccountsDelete)

	// 用户注册/绑定与 SSH 登录校验
	api.GET("/registry/resolve", s.handleRegistryResolve)
	api.GET("/registry/nodes/:node_id/users.txt", s.handleRegistryNodeUsersTxt)
	api.GET("/registry/nodes/:node_id/blocked.txt", s.handleRegistryNodeBlockedUsersTxt)
	api.GET("/registry/nodes/:node_id/exempt.txt", s.handleRegistryNodeExemptUsersTxt)

	// 用户自助登记/开号申请（管理员审核）
	api.GET("/requests", s.handleUserRequestsList)
	api.POST("/requests/bind", s.handleUserBindRequestsCreate)
	api.POST("/requests/open", s.handleUserOpenRequestCreate)

	// 排队接口（可选）：当前实现为“纯排队/不分配”的可运行版本，便于后续接入真实资源分配策略
	api.POST("/gpu/request", s.handleGPURequest)

	admin := api.Group("/admin")
	admin.Use(s.authAdmin())
	admin.POST("/bootstrap", s.requireSuperAdmin(), s.handleAdminBootstrap)
	admin.GET("/users", s.requireSuperAdmin(), s.handleAdminUsers)
	admin.GET("/users/details", s.requireSuperAdmin(), s.handleAdminUserDetails)
	admin.GET("/prices", s.requireSuperAdmin(), s.handleAdminPrices)
	admin.POST("/prices", s.requireSuperAdmin(), s.handleAdminSetPrice)
	admin.GET("/gpu/queue", s.requireSuperAdmin(), s.handleAdminGPUQueue)
	admin.GET("/requests", s.requireReviewPermission(), s.handleAdminRequestsList)
	admin.POST("/requests/:id/approve", s.requireReviewPermission(), s.handleAdminRequestApprove)
	admin.POST("/requests/:id/reject", s.requireReviewPermission(), s.handleAdminRequestReject)
	admin.POST("/requests/batch-review", s.requireReviewPermission(), s.handleAdminRequestsBatchReview)
	admin.GET("/profile-change-requests", s.requireReviewPermission(), s.handleAdminProfileChangeRequestsList)
	admin.POST("/profile-change-requests/:id/approve", s.requireReviewPermission(), s.handleAdminProfileChangeApprove)
	admin.POST("/profile-change-requests/:id/reject", s.requireReviewPermission(), s.handleAdminProfileChangeReject)
	admin.POST("/announcements", s.requireSuperAdmin(), s.handleAnnouncementCreate)
	admin.DELETE("/announcements/:id", s.requireSuperAdmin(), s.handleAnnouncementDelete)
	admin.GET("/usage", s.requireSuperAdmin(), s.handleAdminUsage)
	admin.GET("/nodes", s.requireNodesPermission(), s.handleAdminNodes)
	admin.POST("/nodes/:id/ssh/disconnect-all", s.requireSuperAdmin(), s.handleAdminNodeDisconnectAllSSH)
	admin.GET("/usage/export.csv", s.requireSuperAdmin(), s.handleAdminUsageExportCSV)
	admin.GET("/mail/settings", s.requireSuperAdmin(), s.handleAdminMailSettingsGet)
	admin.POST("/mail/settings", s.requireSuperAdmin(), s.handleAdminMailSettingsSet)
	admin.POST("/mail/test", s.requireSuperAdmin(), s.handleAdminMailTest)
	admin.GET("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsList)
	admin.POST("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsUpsert)
	admin.PUT("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsUpdate)
	admin.DELETE("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsDelete)
	admin.GET("/whitelist", s.requireSuperAdmin(), s.handleAdminWhitelistList)
	admin.POST("/whitelist", s.requireSuperAdmin(), s.handleAdminWhitelistUpsert)
	admin.DELETE("/whitelist", s.requireSuperAdmin(), s.handleAdminWhitelistDelete)
	admin.GET("/blacklist", s.requireSuperAdmin(), s.handleAdminBlacklistList)
	admin.POST("/blacklist", s.requireSuperAdmin(), s.handleAdminBlacklistUpsert)
	admin.DELETE("/blacklist", s.requireSuperAdmin(), s.handleAdminBlacklistDelete)
	admin.GET("/exemptions", s.requireSuperAdmin(), s.handleAdminExemptionsList)
	admin.POST("/exemptions", s.requireSuperAdmin(), s.handleAdminExemptionsUpsert)
	admin.DELETE("/exemptions", s.requireSuperAdmin(), s.handleAdminExemptionsDelete)
	admin.GET("/power-users", s.requireSuperAdmin(), s.handleAdminPowerUsersList)
	admin.POST("/power-users", s.requireSuperAdmin(), s.handleAdminPowerUsersCreate)
	admin.PUT("/power-users/:username/permissions", s.requireSuperAdmin(), s.handleAdminPowerUsersUpdatePermissions)
	admin.DELETE("/power-users/:username", s.requireSuperAdmin(), s.handleAdminPowerUsersDelete)
	admin.GET("/stats/users", s.requireBoardPermission(), s.handleAdminStatsUsers)
	admin.GET("/stats/platform-users", s.requireBoardPermission(), s.handleAdminStatsPlatformUsers)
	admin.GET("/stats/platform-users/:username/nodes", s.requireBoardPermission(), s.handleAdminStatsPlatformUserNodes)
	admin.GET("/stats/monthly", s.requireBoardPermission(), s.handleAdminStatsMonthly)
	admin.GET("/stats/recharges", s.requireBoardPermission(), s.handleAdminStatsRecharges)

	s.maybeServeWeb(r)
	return r
}

func (s *Server) authSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg.SessionHours == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		secret := strings.TrimSpace(s.cfg.AuthSecret)
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		p, err := verifySession(secret, cookie, time.Now())
		if err != nil || strings.TrimSpace(p.Username) == "" || strings.TrimSpace(p.Role) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("auth_user", p.Username)
		c.Set("auth_role", p.Role)
		c.Set("auth_perms", p.Perms)
		c.Set("csrf", p.Nonce)

		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			want := p.Nonce
			got := strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
			if want == "" || got == "" || want != got {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_required"})
				return
			}
		}
		c.Next()
	}
}

func (s *Server) authAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := strings.TrimSpace(c.GetHeader("X-Agent-Token"))
		if tok == "" || tok != s.cfg.AgentToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func (s *Server) authAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) 优先支持脚本类 Bearer admin_token
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) && strings.TrimSpace(strings.TrimPrefix(auth, prefix)) == s.cfg.AdminToken {
			c.Set("auth_method", "token")
			c.Set("auth_role", "admin")
			c.Set("auth_perms", uint32(permViewBoard|permViewNodes|permReviewRequests))
			c.Next()
			return
		}

		// 2) Web 登录会话（cookie）
		if s.cfg.SessionHours == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		secret := strings.TrimSpace(s.cfg.AuthSecret)
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || strings.TrimSpace(cookie) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		p, err := verifySession(secret, cookie, time.Now())
		if err != nil || (p.Role != "admin" && p.Role != "power_user") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("auth_method", "session")
		c.Set("auth_user", p.Username)
		c.Set("auth_role", p.Role)
		c.Set("auth_perms", p.Perms)
		c.Set("csrf", p.Nonce)

		// CSRF：仅对“有副作用”的请求要求 header（GET 不需要）
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			want := p.Nonce
			got := strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
			if want == "" || got == "" || want != got {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_required"})
				return
			}
		}
		c.Next()
	}
}

func getAuthRole(c *gin.Context) string {
	if v, ok := c.Get("auth_role"); ok {
		if s, ok2 := v.(string); ok2 {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func getAuthPerms(c *gin.Context) uint32 {
	if v, ok := c.Get("auth_perms"); ok {
		switch t := v.(type) {
		case uint32:
			return t
		case int:
			if t >= 0 {
				return uint32(t)
			}
		}
	}
	return 0
}

func (s *Server) requireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_method"))) == "token" {
			c.Next()
			return
		}
		if getAuthRole(c) != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func (s *Server) requireBoardPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := getAuthRole(c)
		if role == "admin" {
			c.Next()
			return
		}
		if role == "power_user" && (getAuthPerms(c)&permViewBoard) != 0 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func (s *Server) requireNodesPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := getAuthRole(c)
		if role == "admin" {
			c.Next()
			return
		}
		if role == "power_user" && (getAuthPerms(c)&permViewNodes) != 0 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func (s *Server) requireReviewPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := getAuthRole(c)
		if role == "admin" {
			c.Next()
			return
		}
		if role == "power_user" && (getAuthPerms(c)&permReviewRequests) != 0 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func (s *Server) handleBalance(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}

	ctx := c.Request.Context()
	var u User
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		u, err = s.store.EnsureUserTx(ctx, tx, username, s.cfg.DefaultBalance)
		return err
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": u.Username,
		"balance":  u.Balance,
		"status":   u.Status,
	})
}

func (s *Server) handleUserUsage(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	records, err := s.store.ListUsageByUser(c.Request.Context(), username, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": records})
}

type rechargeReq struct {
	Amount float64 `json:"amount"`
	Method string  `json:"method"`
}

func (s *Server) handleRecharge(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}

	var req rechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	now := time.Now()
	var res BalanceUpdateResult
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		res, err = s.store.RechargeTx(ctx, tx, username, req.Amount, req.Method, now, s.cfg)
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": res.User.Username,
		"balance":  res.User.Balance,
		"status":   res.User.Status,
	})
}

func (s *Server) handleAdminUsers(c *gin.Context) {
	limit := 1000
	users, err := s.store.ListUsers(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (s *Server) handleAdminUserDetails(c *gin.Context) {
	limit := 1000
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.store.ListAdminUserDetails(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": rows})
}

func (s *Server) handleAnnouncementsList(c *gin.Context) {
	limit := 20
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.store.ListAnnouncements(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"announcements": rows})
}

type announcementCreateReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Pinned  bool   `json:"pinned"`
}

func (s *Server) handleAnnouncementCreate(c *gin.Context) {
	var req announcementCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	createdBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if x, ok2 := v.(string); ok2 && strings.TrimSpace(x) != "" {
			createdBy = strings.TrimSpace(x)
		}
	}
	if err := s.store.CreateAnnouncement(c.Request.Context(), req.Title, req.Content, req.Pinned, createdBy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAnnouncementDelete(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	if err := s.store.DeleteAnnouncement(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleUserMe(c *gin.Context) {
	username := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	role := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_role")))
	if role == "admin" || role == "power_user" {
		c.JSON(http.StatusOK, gin.H{
			"username": username,
			"role":     role,
		})
		return
	}
	acc, err := s.store.GetUserAccountByUsername(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, acc)
}

func (s *Server) handleUserMyBalance(c *gin.Context) {
	username := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	u, err := s.store.GetUser(c.Request.Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"username": username, "balance": s.cfg.DefaultBalance, "status": "normal"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"username": u.Username, "balance": u.Balance, "status": u.Status})
}

func (s *Server) handleUserMyUsage(c *gin.Context) {
	username := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	records, err := s.store.ListUsageByUser(c.Request.Context(), username, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": records})
}

type userProfileUpdateReq struct {
	Email                  string `json:"email"`
	Username               string `json:"username"`
	StudentID              string `json:"student_id"`
	RealName               string `json:"real_name"`
	Advisor                string `json:"advisor"`
	ExpectedGraduationYear int    `json:"expected_graduation_year"`
	Phone                  string `json:"phone"`
	ChangeReason           string `json:"change_reason"`
}

func (s *Server) handleUserMyProfileUpdate(c *gin.Context) {
	authUsername := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req userProfileUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	req.StudentID = strings.TrimSpace(req.StudentID)
	req.RealName = strings.TrimSpace(req.RealName)
	req.Advisor = strings.TrimSpace(req.Advisor)
	req.Phone = strings.TrimSpace(req.Phone)
	req.ChangeReason = strings.TrimSpace(req.ChangeReason)
	if req.Email == "" || req.Username == "" || req.StudentID == "" || req.RealName == "" || req.Advisor == "" || req.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请完整填写资料"})
		return
	}
	profile, err := s.store.GetUserAccountByUsername(c.Request.Context(), authUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	needReview := req.Username != profile.Username || req.Email != strings.ToLower(profile.Email) || req.StudentID != profile.StudentID
	if err := s.store.UpdateUserProfileBase(c.Request.Context(), authUsername, req.RealName, req.Advisor, req.ExpectedGraduationYear, req.Phone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if needReview {
		if req.ChangeReason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "修改用户名/邮箱/学号时，请填写变更备注供管理员审核"})
			return
		}
		if err := s.store.CreateProfileChangeRequest(c.Request.Context(), authUsername, req.Username, req.Email, req.StudentID, req.ChangeReason); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"ok":                true,
			"profile_updated":   true,
			"request_submitted": true,
			"message":           "资料已保存；用户名/邮箱/学号变更已提交管理员审核",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                true,
		"profile_updated":   true,
		"request_submitted": false,
		"message":           "资料已更新",
	})
}

func (s *Server) handleUserMyProfileChangeRequests(c *gin.Context) {
	authUsername := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	limit := parseLimit(c.Query("limit"), 100, 1000)
	rows, err := s.store.ListProfileChangeRequestsByUser(c.Request.Context(), authUsername, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": rows})
}

type userAccountUpsertReq struct {
	NodeID        string `json:"node_id"`
	LocalUsername string `json:"local_username"`
}

type userAccountUpdateReq struct {
	OldNodeID        string `json:"old_node_id"`
	OldLocalUsername string `json:"old_local_username"`
	NewNodeID        string `json:"new_node_id"`
	NewLocalUsername string `json:"new_local_username"`
}

func (s *Server) handleUserAccountsList(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	rows, err := s.store.ListUserNodeAccountsByBilling(c.Request.Context(), billing, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": rows})
}

func (s *Server) handleUserAccountsUpsert(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req userAccountUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertUserNodeAccount(c.Request.Context(), req.NodeID, req.LocalUsername, billing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleUserAccountsUpdate(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req userAccountUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpdateUserNodeAccount(c.Request.Context(),
		req.OldNodeID, req.OldLocalUsername, billing,
		req.NewNodeID, req.NewLocalUsername, billing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleUserAccountsDelete(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	if err := s.store.DeleteUserNodeAccount(c.Request.Context(), nodeID, localUsername, billing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type adminAccountUpsertReq struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
}

type adminAccountUpdateReq struct {
	OldBillingUsername string `json:"old_billing_username"`
	OldNodeID          string `json:"old_node_id"`
	OldLocalUsername   string `json:"old_local_username"`
	NewBillingUsername string `json:"new_billing_username"`
	NewNodeID          string `json:"new_node_id"`
	NewLocalUsername   string `json:"new_local_username"`
}

func (s *Server) handleAdminAccountsList(c *gin.Context) {
	billing := strings.TrimSpace(c.Query("billing_username"))
	rows, err := s.store.ListUserNodeAccounts(c.Request.Context(), billing, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": rows})
}

func (s *Server) handleAdminAccountsUpsert(c *gin.Context) {
	var req adminAccountUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertUserNodeAccount(c.Request.Context(), req.NodeID, req.LocalUsername, req.BillingUsername); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminAccountsUpdate(c *gin.Context) {
	var req adminAccountUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpdateUserNodeAccount(c.Request.Context(),
		req.OldNodeID, req.OldLocalUsername, req.OldBillingUsername,
		req.NewNodeID, req.NewLocalUsername, req.NewBillingUsername); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminAccountsDelete(c *gin.Context) {
	billing := strings.TrimSpace(c.Query("billing_username"))
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	if err := s.store.DeleteUserNodeAccount(c.Request.Context(), nodeID, localUsername, billing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type sshListUpsertReq struct {
	NodeID           string   `json:"node_id"`
	Usernames        []string `json:"usernames"`
	BillingUsernames []string `json:"billing_usernames"`
}

type sshListEntry struct {
	NodeID        string
	LocalUsername string
}

func trimUniq(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		v := strings.TrimSpace(it)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (s *Server) currentOperator(c *gin.Context) string {
	operator := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if x, ok2 := v.(string); ok2 && strings.TrimSpace(x) != "" {
			operator = strings.TrimSpace(x)
		}
	}
	return operator
}

func (s *Server) resolveSSHListEntries(ctx context.Context, req sshListUpsertReq) ([]sshListEntry, error) {
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	manualUsers := trimUniq(req.Usernames)
	billingUsers := trimUniq(req.BillingUsernames)
	if len(manualUsers) == 0 && len(billingUsers) == 0 {
		return nil, errors.New("usernames/billing_usernames 至少填写一项")
	}

	entries := map[string]sshListEntry{}
	addEntry := func(node, local string) {
		node = strings.TrimSpace(node)
		local = strings.TrimSpace(local)
		if node == "" || local == "" {
			return
		}
		k := node + "|" + local
		entries[k] = sshListEntry{NodeID: node, LocalUsername: local}
	}

	for _, local := range manualUsers {
		if nodeID == "*" {
			addEntry("*", local)
		} else {
			addEntry(nodeID, local)
		}
	}

	for _, billing := range billingUsers {
		accounts, err := s.store.ListUserNodeAccountsByBilling(ctx, billing, 5000)
		if err != nil {
			return nil, err
		}
		matched := 0
		for _, acc := range accounts {
			accNode := strings.TrimSpace(acc.NodeID)
			accLocal := strings.TrimSpace(acc.LocalUsername)
			if accNode == "" || accLocal == "" {
				continue
			}
			if nodeID != "*" && accNode != nodeID {
				continue
			}
			addEntry(accNode, accLocal)
			matched++
		}
		if matched == 0 {
			if nodeID == "*" {
				addEntry("*", billing)
			} else {
				addEntry(nodeID, billing)
			}
		}
	}

	out := make([]sshListEntry, 0, len(entries))
	for _, v := range entries {
		out = append(out, v)
	}
	return out, nil
}

func (s *Server) upsertWhitelistEntries(ctx context.Context, entries []sshListEntry, createdBy string) error {
	grouped := map[string][]string{}
	for _, e := range entries {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertWhitelist(ctx, nodeID, trimUniq(users), createdBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) upsertBlacklistEntries(ctx context.Context, entries []sshListEntry, createdBy string) error {
	grouped := map[string][]string{}
	for _, e := range entries {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertBlacklist(ctx, nodeID, trimUniq(users), createdBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) upsertExemptionEntries(ctx context.Context, entries []sshListEntry, createdBy string) error {
	grouped := map[string][]string{}
	for _, e := range entries {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertExemptions(ctx, nodeID, trimUniq(users), createdBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) enqueueKickSSHUser(ctx context.Context, nodeID string, localUsername string, reason string) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return
	}
	targets := []string{}
	if nodeID == "*" {
		nodes, err := s.store.ListNodes(ctx, 5000)
		if err == nil {
			seen := map[string]struct{}{}
			for _, n := range nodes {
				id := strings.TrimSpace(n.NodeID)
				if id == "" {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				targets = append(targets, id)
			}
		}
	} else {
		targets = append(targets, nodeID)
	}
	for _, id := range targets {
		exempted, err := s.store.IsExempted(ctx, id, localUsername)
		if err == nil && exempted {
			continue
		}
		s.enqueueNodeAction(id, Action{
			Type:     "kick_ssh_user",
			Username: localUsername,
			Reason:   reason,
		})
	}
}

func (s *Server) handleAdminWhitelistList(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	rows, err := s.store.ListWhitelist(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows})
}

func (s *Server) handleAdminWhitelistUpsert(c *gin.Context) {
	var req sshListUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entries, err := s.resolveSSHListEntries(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.upsertWhitelistEntries(c.Request.Context(), entries, s.currentOperator(c)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "entries": len(entries)})
}

func (s *Server) handleAdminWhitelistDelete(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	nodes, err := s.store.DeleteWhitelistWithNodes(c.Request.Context(), nodeID, localUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := s.currentOperator(c)
	for _, n := range nodes {
		s.enqueueKickSSHUser(c.Request.Context(), n, localUsername, fmt.Sprintf("管理员 %s 删除白名单，已强制断开该账号 SSH 会话", operator))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "kicked": true})
}

func (s *Server) handleAdminBlacklistList(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	rows, err := s.store.ListBlacklist(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows})
}

func (s *Server) handleAdminBlacklistUpsert(c *gin.Context) {
	var req sshListUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entries, err := s.resolveSSHListEntries(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := s.currentOperator(c)
	if err := s.upsertBlacklistEntries(c.Request.Context(), entries, operator); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, e := range entries {
		s.enqueueKickSSHUser(c.Request.Context(), e.NodeID, e.LocalUsername, fmt.Sprintf("管理员 %s 加入 SSH 黑名单，已强制断开该账号 SSH 会话", operator))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "entries": len(entries), "kicked": true})
}

func (s *Server) handleAdminBlacklistDelete(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	if err := s.store.DeleteBlacklist(c.Request.Context(), nodeID, localUsername); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminExemptionsList(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	rows, err := s.store.ListExemptions(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows})
}

func (s *Server) handleAdminExemptionsUpsert(c *gin.Context) {
	var req sshListUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entries, err := s.resolveSSHListEntries(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := s.currentOperator(c)
	if err := s.upsertExemptionEntries(c.Request.Context(), entries, operator); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "entries": len(entries)})
}

func (s *Server) handleAdminExemptionsDelete(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	if err := s.store.DeleteExemptions(c.Request.Context(), nodeID, localUsername); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type powerUserCreateReq struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	CanViewBoard      bool   `json:"can_view_board"`
	CanViewNodes      bool   `json:"can_view_nodes"`
	CanReviewRequests bool   `json:"can_review_requests"`
}

type powerUserPermReq struct {
	CanViewBoard      bool `json:"can_view_board"`
	CanViewNodes      bool `json:"can_view_nodes"`
	CanReviewRequests bool `json:"can_review_requests"`
}

func (s *Server) handleAdminPowerUsersList(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 1000, 5000)
	rows, err := s.store.ListPowerUsers(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": rows})
}

func (s *Server) handleAdminPowerUsersCreate(c *gin.Context) {
	var req powerUserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	createdBy := strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_user")))
	if createdBy == "" {
		createdBy = "admin"
	}
	if err := s.store.CreatePowerUser(c.Request.Context(), req.Username, req.Password, req.CanViewBoard, req.CanViewNodes, req.CanReviewRequests, createdBy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminPowerUsersUpdatePermissions(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	var req powerUserPermReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updatedBy := strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_user")))
	if updatedBy == "" {
		updatedBy = "admin"
	}
	if err := s.store.UpdatePowerUserPermissions(c.Request.Context(), username, req.CanViewBoard, req.CanViewNodes, req.CanReviewRequests, updatedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminPowerUsersDelete(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	if err := s.store.DeletePowerUser(c.Request.Context(), username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminPrices(c *gin.Context) {
	prices, err := s.store.ListPrices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

type setPriceReq struct {
	GPUModel       string  `json:"gpu_model"`
	PricePerMinute float64 `json:"price_per_minute"`
}

func (s *Server) handleAdminSetPrice(c *gin.Context) {
	var req setPriceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertPrice(c.Request.Context(), req.GPUModel, req.PricePerMinute); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminUsage(c *gin.Context) {
	billingUsername := strings.TrimSpace(c.Query("billing_username"))
	if billingUsername == "" {
		// 兼容旧参数
		billingUsername = strings.TrimSpace(c.Query("username"))
	}
	localUsername := strings.TrimSpace(c.Query("local_username"))
	unregisteredOnly := strings.TrimSpace(c.Query("unregistered_only")) == "1"
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	records, err := s.store.ListUsageAdmin(c.Request.Context(), billingUsername, localUsername, unregisteredOnly, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": records})
}

func (s *Server) handleAdminStatsUsers(c *gin.Context) {
	from, to, err := parseStatsRange(c, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := parseLimit(c.Query("limit"), 1000, 10000)
	rows, err := s.store.ListUsageSummaryByUser(c.Request.Context(), from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": rows})
}

func (s *Server) handleAdminStatsPlatformUsers(c *gin.Context) {
	from, to, err := parseStatsRange(c, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := parseLimit(c.Query("limit"), 1000, 10000)
	rows, err := s.store.ListPlatformUsageSummaryByUser(c.Request.Context(), from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": rows})
}

func (s *Server) handleAdminStatsPlatformUserNodes(c *gin.Context) {
	from, to, err := parseStatsRange(c, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	limit := parseLimit(c.Query("limit"), 2000, 20000)
	rows, err := s.store.ListPlatformUsageNodeDetails(c.Request.Context(), username, from, to, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "username": username, "rows": rows})
}

func (s *Server) handleAdminProfileChangeRequestsList(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	username := strings.TrimSpace(c.Query("username"))
	limit := parseLimit(c.Query("limit"), 500, 5000)
	rows, err := s.store.ListProfileChangeRequestsAdmin(c.Request.Context(), status, username, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": rows})
}

func (s *Server) handleAdminProfileChangeApprove(c *gin.Context) {
	s.handleAdminProfileChangeReview(c, "approved")
}

func (s *Server) handleAdminProfileChangeReject(c *gin.Context) {
	s.handleAdminProfileChangeReview(c, "rejected")
}

func (s *Server) handleAdminProfileChangeReview(c *gin.Context, newStatus string) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
			reviewedBy = strings.TrimSpace(s)
		}
	}
	var out ProfileChangeRequest
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var err error
		out, err = s.store.ReviewProfileChangeRequestTx(c.Request.Context(), tx, id, newStatus, reviewedBy, time.Now())
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "request": out})
}

func (s *Server) handleAdminStatsMonthly(c *gin.Context) {
	from, to, err := parseStatsRange(c, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := parseLimit(c.Query("limit"), 20000, 200000)
	rows, err := s.store.ListUsageMonthlyByUser(c.Request.Context(), from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": rows})
}

func (s *Server) handleAdminStatsRecharges(c *gin.Context) {
	from, to, err := parseStatsRange(c, 365)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := parseLimit(c.Query("limit"), 1000, 10000)
	rows, err := s.store.ListRechargeSummary(c.Request.Context(), from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": rows})
}

func (s *Server) handleAdminNodes(c *gin.Context) {
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	nodes, err := s.store.ListNodes(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

func (s *Server) handleAdminNodeDisconnectAllSSH(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	node, err := s.store.GetNodeStatus(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operator := strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_user")))
	if operator == "" {
		operator = "admin"
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:   "kick_ssh_all",
		Reason: fmt.Sprintf("管理员 %s 发起：清理节点 SSH 会话", operator),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"node_id":          nodeID,
		"ssh_active_count": node.SSHActiveCount,
		"message":          "已下发清理指令，节点会在约 1 秒内执行",
		"requested_by":     operator,
		"requested_at":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminUsageExportCSV(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))
	limit := 20000
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit <= 0 || limit > 200000 {
		limit = 20000
	}

	var from time.Time
	var to time.Time
	var err error
	if fromStr != "" {
		from, err = parseTimeFlexible(fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD"})
			return
		}
	}
	if toStr != "" {
		to, err = parseTimeFlexible(toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD"})
			return
		}
	}

	ctx := c.Request.Context()
	rows, err := s.store.queryUsageRows(ctx, username, fromStr != "", from, toStr != "", to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	filename := "usage_export.csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"timestamp", "node_id", "billing_username", "local_username", "cpu_percent", "memory_mb", "cost", "gpu_usage_json"})

	for rows.Next() {
		var nodeID, user, localUsername string
		var ts time.Time
		var cpuPercent, memoryMB, cost float64
		var gpuUsage string
		if err := rows.Scan(&nodeID, &user, &ts, &cpuPercent, &memoryMB, &gpuUsage, &cost, &localUsername); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		_ = w.Write([]string{
			ts.Format(time.RFC3339),
			nodeID,
			user,
			localUsername,
			fmt.Sprintf("%.4f", cpuPercent),
			fmt.Sprintf("%.4f", memoryMB),
			fmt.Sprintf("%.4f", cost),
			gpuUsage,
		})
	}
	w.Flush()
}

type adminMailTestReq struct {
	Username string `json:"username"`
}

func (s *Server) handleAdminMailTest(c *gin.Context) {
	var req adminMailTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	email, err := s.store.GetUserEmailByUsername(c.Request.Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该用户没有注册邮箱"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	subject := "GPU Ops 邮件配置测试"
	body := fmt.Sprintf("你好 %s，\n\n这是一封测试邮件，表示管理员已成功配置 SMTP。\n时间：%s\n\nGPU Ops 团队", username, time.Now().Format(time.RFC3339))
	if err := sendResetPasswordMail(settings, email, subject, body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "发送失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "email": email})
}

func (s *Server) handleMetrics(c *gin.Context) {
	var data MetricsData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data.NodeID = strings.TrimSpace(data.NodeID)
	if data.NodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	data.ReportID = strings.TrimSpace(data.ReportID)
	if data.ReportID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "report_id 不能为空（用于幂等防重）"})
		return
	}

	reportTS, err := time.Parse(time.RFC3339, strings.TrimSpace(data.Timestamp))
	if err != nil {
		// 允许 Agent 不传或传错时间，控制器兜底为当前时间
		reportTS = time.Now()
	}

	// 先做轻量清洗：去掉无效记录，避免污染账单
	cleaned := make([]UserProcess, 0, len(data.Users))
	for _, p := range data.Users {
		p.Username = strings.TrimSpace(p.Username)
		if p.Username == "" || p.PID <= 0 {
			continue
		}
		// CPU-only 进程也允许进入：后续按 CPUPercent 决定是否计费
		cleaned = append(cleaned, p)
	}
	data.Users = cleaned

	ctx := c.Request.Context()
	actions, err := s.processMetrics(ctx, data, reportTS)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ControllerResponse{Actions: actions})
}

func (s *Server) handleNodeActions(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	actions := s.popNodeActions(nodeID)
	c.JSON(http.StatusOK, ControllerResponse{Actions: actions})
}

type gpuRequestReq struct {
	Username string `json:"username"`
	GPUType  string `json:"gpu_type"`
	Count    int    `json:"count"`
}

func (s *Server) handleGPURequest(c *gin.Context) {
	var req gpuRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.GPUType = strings.TrimSpace(req.GPUType)
	if req.Username == "" || req.GPUType == "" || req.Count <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/gpu_type/count 参数不合法"})
		return
	}

	item := QueueItem{
		Username:  req.Username,
		GPUType:   req.GPUType,
		Count:     req.Count,
		Timestamp: time.Now(),
	}

	pos := s.queue.Enqueue(item)
	estimated := estimateWaitMinutes(pos)

	c.JSON(http.StatusOK, gin.H{
		"status":            "queued",
		"position":          pos,
		"estimated_minutes": estimated,
		"message":           "当前无可用 GPU，已加入排队",
	})
}

func (s *Server) handleAdminGPUQueue(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"queue": s.queue.Snapshot()})
}

func (s *Server) processMetrics(ctx context.Context, data MetricsData, reportTS time.Time) ([]Action, error) {
	now := time.Now()
	grace := time.Duration(s.cfg.KillGracePeriodSeconds) * time.Second
	intervalSeconds := s.cfg.SampleIntervalSeconds
	if data.IntervalSeconds > 0 && data.IntervalSeconds <= 600 {
		intervalSeconds = data.IntervalSeconds
	}
	intervalMinutes := float64(intervalSeconds) / 60.0
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	type localAgg struct {
		pids []int32
	}
	type billingAgg struct {
		cost   float64
		locals map[string]*localAgg // local_username -> pids
	}
	billingAggs := make(map[string]*billingAgg)
	usageRecords := 0
	gpuProcCount := 0
	cpuProcCount := 0
	costTotal := 0.0

	var actions []Action
	duplicate := false

	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		inserted, err := s.store.TryInsertReportTx(ctx, tx, data.ReportID, data.NodeID, reportTS, intervalSeconds)
		if err != nil {
			return err
		}
		if !inserted {
			duplicate = true
			return nil
		}

		priceRows, err := s.store.LoadPricesTx(ctx, tx)
		if err != nil {
			return err
		}
		priceIndex := NewPriceIndex(priceRows)
		cpuPricePerCoreMinute := s.cfg.CPUPricePerCoreMinute
		if v, ok := priceIndex.MatchPrice("CPU_CORE"); ok {
			cpuPricePerCoreMinute = v
		}

		// 同一台节点的映射在一次上报内复用，避免对每个进程重复查库
		resolveCache := make(map[string]string) // local_username -> billing_username（未绑定时为自身）

		for _, proc := range data.Users {
			localUsername := strings.TrimSpace(proc.Username)
			if localUsername == "" {
				continue
			}

			billingUsername, ok := resolveCache[localUsername]
			if !ok {
				mapped, found, err := s.store.ResolveBillingUsernameTx(ctx, tx, data.NodeID, localUsername)
				if err != nil {
					return err
				}
				if found && strings.TrimSpace(mapped) != "" {
					billingUsername = mapped
				} else {
					billingUsername = localUsername
				}
				resolveCache[localUsername] = billingUsername
			}

			gpuCost := 0.0
			if len(proc.GPUUsage) > 0 {
				gpuCost = CalculateProcessCost(proc, priceIndex, s.cfg.DefaultPricePerMinute)
			}
			proc.Command = strings.TrimSpace(proc.Command)
			if len(proc.Command) > 256 {
				proc.Command = proc.Command[:256]
			}
			cpuCost := (proc.CPUPercent / 100.0) * cpuPricePerCoreMinute * intervalMinutes
			cost := round4(gpuCost + cpuCost)

			// 如果既没有 GPU，也几乎不占 CPU，就不计费也不落库（避免噪声与膨胀）
			if len(proc.GPUUsage) == 0 && proc.CPUPercent < 1.0 {
				continue
			}
			// usage_records 归集到计费账号，便于按“中心账号”对账/查询
			procForStore := proc
			procForStore.Username = billingUsername
			if err := s.store.InsertUsageRecordTx(ctx, tx, data.NodeID, localUsername, reportTS, procForStore, cost); err != nil {
				return err
			}
			usageRecords++
			costTotal += cost
			if len(proc.GPUUsage) > 0 {
				gpuProcCount++
			} else {
				cpuProcCount++
			}

			b := billingAggs[billingUsername]
			if b == nil {
				b = &billingAgg{locals: make(map[string]*localAgg)}
				billingAggs[billingUsername] = b
			}
			b.cost += cost
			la := b.locals[localUsername]
			if la == nil {
				la = &localAgg{}
				b.locals[localUsername] = la
			}
			la.pids = append(la.pids, proc.PID)
		}

		for billingUsername, b := range billingAggs {
			res, err := s.store.DeductBalanceTx(ctx, tx, billingUsername, b.cost, now, s.cfg)
			if err != nil {
				return err
			}

			// 注意：扣费与余额状态以“计费账号”为准；但下发动作必须针对“节点本地账号”，否则 Agent 无法生效。
			for localUsername, la := range b.locals {
				uLocal := res.User
				uLocal.Username = localUsername
				actions = append(actions, DecideActions(now, res.PrevStatus, uLocal, s.cfg.WarningThreshold, s.cfg.LimitedThreshold, grace, la.pids)...)

				if s.cfg.EnableCPUControl {
					if res.User.Status == "limited" {
						actions = append(actions, Action{
							Type:            "set_cpu_quota",
							Username:        localUsername,
							CPUQuotaPercent: s.cfg.CPULimitPercentLimited,
							Reason:          "余额不足，限制 CPU 使用",
						})
					} else if res.User.Status == "blocked" {
						actions = append(actions, Action{
							Type:            "set_cpu_quota",
							Username:        localUsername,
							CPUQuotaPercent: s.cfg.CPULimitPercentBlocked,
							Reason:          "已欠费，强限制 CPU 使用",
						})
					} else if res.PrevStatus == "limited" || res.PrevStatus == "blocked" {
						actions = append(actions, Action{
							Type:            "set_cpu_quota",
							Username:        localUsername,
							CPUQuotaPercent: 0,
							Reason:          "余额已恢复，解除 CPU 限制",
						})
					}
				}
			}
		}

		// 更新节点状态（用于运维查看在线/上报情况）
		if err := s.store.UpsertNodeStatusTx(
			ctx,
			tx,
			data.NodeID,
			now,
			data.ReportID,
			reportTS,
			intervalSeconds,
			data.CPUModel,
			data.CPUCount,
			data.GPUModel,
			data.GPUCount,
			data.NetRxBytes,
			data.NetTxBytes,
			gpuProcCount,
			cpuProcCount,
			usageRecords,
			len(data.SSHUsers),
			round4(costTotal),
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if duplicate {
		pending := s.popNodeActions(data.NodeID)
		s.metr.observeReport(now, true, 0, pending)
		return pending, nil
	}
	actions = append(actions, s.popNodeActions(data.NodeID)...)
	s.metr.observeReport(now, false, usageRecords, actions)
	return actions, nil
}

func round4(v float64) float64 {
	// 避免引入更多依赖，使用 billing.go 同样的舍入策略
	return float64(int64(v*10000+0.5)) / 10000
}

func parseTimeFlexible(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	// RFC3339
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	// YYYY-MM-DD（按 UTC 00:00:00）
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	// 兼容常见：YYYY-MM-DD HH:MM:SS（按本地时间）
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time: %s", v)
}

func parseStatsRange(c *gin.Context, defaultDays int) (time.Time, time.Time, error) {
	now := time.Now()
	from := now.AddDate(0, 0, -defaultDays)
	to := now
	if x := strings.TrimSpace(c.Query("from")); x != "" {
		t, err := parseTimeFlexible(x)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("from 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD")
		}
		from = t
	}
	if x := strings.TrimSpace(c.Query("to")); x != "" {
		t, err := parseTimeFlexible(x)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("to 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD")
		}
		if len(x) == len("2006-01-02") {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		to = t
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("to 不能早于 from")
	}
	return from, to, nil
}

func parseLimit(v string, def int, max int) int {
	n := def
	if x := strings.TrimSpace(v); x != "" {
		if y, err := strconv.Atoi(x); err == nil {
			n = y
		}
	}
	if n <= 0 {
		n = def
	}
	if n > max {
		n = max
	}
	return n
}

func (s *Server) maybeServeWeb(r *gin.Engine) {
	webDir := strings.TrimSpace(s.cfg.WebDir)
	if webDir == "" {
		candidates := []string{
			filepath.FromSlash("../web/dist"),
			filepath.FromSlash("web/dist"),
		}
		for _, p := range candidates {
			if dirExists(p) {
				webDir = p
				break
			}
		}
	}
	if webDir == "" || !dirExists(webDir) {
		return
	}
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
		return
	}

	// 静态资源直出（index 交给 NoRoute）
	if dirExists(filepath.Join(webDir, "static")) {
		r.Static("/static", filepath.Join(webDir, "static"))
	}

	// 只在 /api 不匹配时回退到 index.html，避免覆盖 API。
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		// 优先直出 dist 根目录下的静态文件（如 /logo.svg、/favicon.ico、/manifest.webmanifest）。
		// 否则浏览器请求图标会被回退到 index.html，导致标签页图标不生效。
		reqPath := strings.TrimPrefix(c.Request.URL.Path, "/")
		if reqPath != "" && !strings.Contains(reqPath, "..") {
			fp := filepath.Join(webDir, filepath.FromSlash(reqPath))
			if info, err := os.Stat(fp); err == nil && !info.IsDir() {
				c.File(fp)
				return
			}
		}

		c.File(filepath.Join(webDir, "index.html"))
	})
}
