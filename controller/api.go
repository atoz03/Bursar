package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

	nodeActionsMu      sync.Mutex
	nodeActions        map[string][]Action
	pointsResetMu      sync.Mutex
	cpuQuotaMu         sync.Mutex
	cpuQuotaState      map[string]float64 // key: node_id::local_username
	memoryLimitMu      sync.Mutex
	memoryLimitState   map[string]float64 // key: node_id::local_username, value=memory_limit_gb
	gpuAccessMu        sync.Mutex
	gpuAccessState     map[string]bool // key: node_id::local_username, true=blocked
	securitySignalMu   sync.Mutex
	securitySignalSeen map[string]securitySignalState // key: node_id::event_type
}

type securitySignalState struct {
	Active       bool
	LastActiveAt time.Time
}

const (
	permViewBoard      = 1
	permViewNodes      = 2
	permReviewRequests = 4
	permManageNodes    = 8
)

func NewServer(cfg Config, store *Store) *Server {
	return &Server{
		cfg:                cfg,
		store:              store,
		queue:              NewQueue(),
		metr:               &controllerMetrics{},
		nodeActions:        make(map[string][]Action),
		cpuQuotaState:      make(map[string]float64),
		memoryLimitState:   make(map[string]float64),
		gpuAccessState:     make(map[string]bool),
		securitySignalSeen: make(map[string]securitySignalState),
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

func quotaStateKey(nodeID, localUsername string) string {
	return strings.TrimSpace(nodeID) + "::" + strings.TrimSpace(localUsername)
}

func securitySignalStateKey(nodeID, eventType string) string {
	return strings.TrimSpace(nodeID) + "::" + strings.TrimSpace(eventType)
}

// 边沿触发 + 恢复静默期：仅在“由正常变为异常”时告警；异常结束后需静默一段时间才认为真正恢复。
func (s *Server) shouldEmitSecuritySignalEvent(
	nodeID string,
	eventType string,
	active bool,
	observedAt time.Time,
	clearHold time.Duration,
) bool {
	nodeID = strings.TrimSpace(nodeID)
	eventType = strings.TrimSpace(eventType)
	if nodeID == "" || eventType == "" {
		return false
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if clearHold < 0 {
		clearHold = 0
	}
	key := securitySignalStateKey(nodeID, eventType)
	s.securitySignalMu.Lock()
	defer s.securitySignalMu.Unlock()
	prev, ok := s.securitySignalSeen[key]
	if active {
		if ok && prev.Active {
			prev.LastActiveAt = observedAt
			s.securitySignalSeen[key] = prev
			return false
		}
		s.securitySignalSeen[key] = securitySignalState{
			Active:       true,
			LastActiveAt: observedAt,
		}
		return true
	}
	if !ok || !prev.Active {
		return false
	}
	if clearHold > 0 {
		if observedAt.Sub(prev.LastActiveAt) < clearHold {
			return false
		}
	}
	delete(s.securitySignalSeen, key)
	return false
}

func (s *Server) nextCPUQuotaAction(nodeID, localUsername string, quotaPercent float64, reason string, force bool) (Action, bool) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return Action{}, false
	}
	key := quotaStateKey(nodeID, localUsername)
	s.cpuQuotaMu.Lock()
	prev, hadPrev := s.cpuQuotaState[key]
	if !force {
		if quotaPercent <= 0 && !hadPrev {
			s.cpuQuotaMu.Unlock()
			return Action{}, false
		}
		if hadPrev && prev == quotaPercent {
			s.cpuQuotaMu.Unlock()
			return Action{}, false
		}
	}
	if quotaPercent <= 0 {
		delete(s.cpuQuotaState, key)
	} else {
		s.cpuQuotaState[key] = quotaPercent
	}
	s.cpuQuotaMu.Unlock()
	return Action{
		Type:            "set_cpu_quota",
		Username:        localUsername,
		CPUQuotaPercent: quotaPercent,
		Reason:          reason,
	}, true
}

func (s *Server) enqueueCPUQuotaAction(nodeID, localUsername string, quotaPercent float64, reason string, force bool) bool {
	action, ok := s.nextCPUQuotaAction(nodeID, localUsername, quotaPercent, reason, force)
	if !ok {
		return false
	}
	s.enqueueNodeAction(nodeID, action)
	return true
}

func (s *Server) nextMemoryLimitAction(nodeID, localUsername string, limitGB float64, reason string, force bool) (Action, bool) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return Action{}, false
	}
	key := quotaStateKey(nodeID, localUsername)
	s.memoryLimitMu.Lock()
	prev, hadPrev := s.memoryLimitState[key]
	if !force {
		if limitGB <= 0 && !hadPrev {
			s.memoryLimitMu.Unlock()
			return Action{}, false
		}
		if hadPrev && prev == limitGB {
			s.memoryLimitMu.Unlock()
			return Action{}, false
		}
	}
	if limitGB <= 0 {
		delete(s.memoryLimitState, key)
	} else {
		s.memoryLimitState[key] = limitGB
	}
	s.memoryLimitMu.Unlock()
	return Action{
		Type:          "set_memory_limit",
		Username:      localUsername,
		MemoryLimitGB: limitGB,
		Reason:        reason,
	}, true
}

func (s *Server) nextGPUAccessAction(nodeID, localUsername string, blocked bool, reason string, force bool) (Action, bool) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	if nodeID == "" || localUsername == "" {
		return Action{}, false
	}
	key := quotaStateKey(nodeID, localUsername)
	s.gpuAccessMu.Lock()
	prev, hadPrev := s.gpuAccessState[key]
	if !force {
		if !blocked && !hadPrev {
			s.gpuAccessMu.Unlock()
			return Action{}, false
		}
		if hadPrev && prev == blocked {
			s.gpuAccessMu.Unlock()
			return Action{}, false
		}
	}
	if blocked {
		s.gpuAccessState[key] = true
	} else {
		delete(s.gpuAccessState, key)
	}
	s.gpuAccessMu.Unlock()
	actionType := "unblock_user"
	if blocked {
		actionType = "block_user"
	}
	return Action{
		Type:     actionType,
		Username: localUsername,
		Reason:   reason,
	}, true
}

func (s *Server) clearNodeCPUQuotaState(nodeID string) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return
	}
	prefix := nodeID + "::"
	s.cpuQuotaMu.Lock()
	for k := range s.cpuQuotaState {
		if strings.HasPrefix(k, prefix) {
			delete(s.cpuQuotaState, k)
		}
	}
	s.cpuQuotaMu.Unlock()
}

func (s *Server) resolveNodeQuotaPolicyFromStatus(node NodeStatus) (enabled bool, threshold float64, limited float64, blocked float64) {
	enabled = node.PointsInterceptEnabled
	threshold = s.cfg.LimitedThreshold
	limited = s.cfg.CPULimitPercentLimited
	blocked = s.cfg.CPULimitPercentBlocked
	if node.PointsThrottleThreshold != nil {
		threshold = *node.PointsThrottleThreshold
	}
	if node.PointsLimitedCPUQuota != nil {
		limited = *node.PointsLimitedCPUQuota
	}
	if node.PointsBlockedCPUQuota != nil {
		blocked = *node.PointsBlockedCPUQuota
	}
	return
}

func (s *Server) resolveNodeMemoryPolicyFromStatus(node NodeStatus) (enabled bool, overdraftMemoryLimitGB float64) {
	enabled = node.PointsInterceptEnabled
	overdraftMemoryLimitGB = s.cfg.OverdraftMemoryLimitGB
	if node.PointsOverdraftMemoryGB != nil {
		overdraftMemoryLimitGB = *node.PointsOverdraftMemoryGB
	}
	if overdraftMemoryLimitGB < 0 {
		overdraftMemoryLimitGB = 0
	}
	return
}

func (s *Server) refreshCPUQuotaForBillingUser(ctx context.Context, billingUsername string, effectiveBalance float64, reasonPrefix string) (int, error) {
	if !s.cfg.EnableCPUControl {
		return 0, nil
	}
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	targets, err := s.listBillingUserTargets(ctx, billingUsername)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	nodes, err := s.store.ListNodes(ctx, 5000)
	if err != nil {
		return 0, err
	}
	nodeByID := make(map[string]NodeStatus, len(nodes))
	for _, n := range nodes {
		nodeByID[n.NodeID] = n
	}
	reasonPrefix = strings.TrimSpace(reasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "管理员操作后重新评估 CPU 限速"
	}
	queued := 0
	for _, t := range targets {
		node, ok := nodeByID[t.NodeID]
		if !ok {
			continue
		}
		enabled, threshold, limitedQuota, blockedQuota := s.resolveNodeQuotaPolicyFromStatus(node)
		quota := 0.0
		reason := reasonPrefix
		switch {
		case !enabled:
			quota = 0
			reason = reasonPrefix + "：节点积分拦截关闭，解除限速"
		case effectiveBalance <= 0:
			quota = blockedQuota
			reason = fmt.Sprintf("%s：余额 %.2f<=0，执行欠费强限速（%.2f%%）", reasonPrefix, effectiveBalance, blockedQuota)
		case effectiveBalance <= threshold:
			quota = limitedQuota
			reason = fmt.Sprintf("%s：余额 %.2f<=阈值 %.2f，执行低积分限速（%.2f%%）", reasonPrefix, effectiveBalance, threshold, limitedQuota)
		default:
			quota = 0
			reason = fmt.Sprintf("%s：余额 %.2f>阈值 %.2f，解除限速", reasonPrefix, effectiveBalance, threshold)
		}
		if s.enqueueCPUQuotaAction(t.NodeID, t.LocalUsername, quota, reason, true) {
			queued++
		}
	}
	return queued, nil
}

func (s *Server) refreshGPUAccessForBillingUser(ctx context.Context, billingUsername string, effectiveBalance float64, reasonPrefix string) (int, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	targets, err := s.listBillingUserTargets(ctx, billingUsername)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	reasonPrefix = strings.TrimSpace(reasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "管理员操作后重新评估 GPU 可见性"
	}
	monthlyCfg, err := s.store.GetMonthlyPointsConfig(ctx)
	if err != nil {
		return 0, err
	}
	maxOverdraftLimit := normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit)
	blockGPU := isOverdraftExceeded(effectiveBalance, maxOverdraftLimit)
	actionType := "unblock_user"
	actionReason := fmt.Sprintf(
		"%s：余额 %.2f 未超过欠费上限 %.2f，恢复 GPU 可见性",
		reasonPrefix,
		effectiveBalance,
		maxOverdraftLimit,
	)
	if blockGPU {
		actionType = "block_user"
		actionReason = fmt.Sprintf(
			"%s：余额 %.2f 已超过每月欠费上限 %.2f（欠费阈值 %.2f），加入欠费 GPU 限制组并隐藏 GPU",
			reasonPrefix,
			effectiveBalance,
			maxOverdraftLimit,
			killAllProcessBalanceThreshold(maxOverdraftLimit),
		)
	}

	queued := 0
	for _, t := range targets {
		blocked := actionType == "block_user"
		action, ok := s.nextGPUAccessAction(t.NodeID, t.LocalUsername, blocked, actionReason, true)
		if !ok {
			continue
		}
		s.enqueueNodeAction(t.NodeID, action)
		queued++
	}
	return queued, nil
}

func (s *Server) refreshMemoryLimitForBillingUser(ctx context.Context, billingUsername string, effectiveBalance float64, reasonPrefix string) (int, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	targets, err := s.listBillingUserTargets(ctx, billingUsername)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	nodes, err := s.store.ListNodes(ctx, 5000)
	if err != nil {
		return 0, err
	}
	nodeByID := make(map[string]NodeStatus, len(nodes))
	for _, n := range nodes {
		nodeByID[n.NodeID] = n
	}
	reasonPrefix = strings.TrimSpace(reasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "管理员操作后重新评估内存限额"
	}
	monthlyCfg, err := s.store.GetMonthlyPointsConfig(ctx)
	if err != nil {
		return 0, err
	}
	maxOverdraftLimit := normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit)
	overdraftExceeded := isOverdraftExceeded(effectiveBalance, maxOverdraftLimit)
	queued := 0
	for _, t := range targets {
		node, ok := nodeByID[t.NodeID]
		if !ok {
			continue
		}
		nodePolicyEnabled, effectiveMemoryLimitGB := s.resolveNodeMemoryPolicyFromStatus(node)
		targetLimitGB := 0.0
		actionReason := reasonPrefix + "：节点积分拦截关闭，解除内存限额"
		switch {
		case !nodePolicyEnabled:
			targetLimitGB = 0
		case overdraftExceeded && effectiveMemoryLimitGB > 0:
			targetLimitGB = effectiveMemoryLimitGB
			actionReason = fmt.Sprintf("%s：余额 %.2f 已超过每月欠费上限 %.2f，内存上限设为 %.2f GB", reasonPrefix, effectiveBalance, maxOverdraftLimit, targetLimitGB)
		default:
			targetLimitGB = 0
			actionReason = fmt.Sprintf("%s：余额 %.2f 未超过每月欠费上限 %.2f，解除内存限额", reasonPrefix, effectiveBalance, maxOverdraftLimit)
		}
		action, ok := s.nextMemoryLimitAction(t.NodeID, t.LocalUsername, targetLimitGB, actionReason, true)
		if !ok {
			continue
		}
		s.enqueueNodeAction(t.NodeID, action)
		queued++
	}
	return queued, nil
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
	api.GET("/auth/register/captcha", s.handleAuthRegisterCaptcha)
	api.GET("/auth/register/check", s.handleAuthRegisterCheck)
	api.GET("/auth/register/verify", s.handleAuthRegisterVerify)
	api.POST("/auth/forgot-password", s.handleAuthForgotPassword)
	api.POST("/auth/reset-password", s.handleAuthResetPassword)
	api.POST("/auth/change-password", s.authSession(), s.handleAuthChangePassword)
	api.GET("/announcements", s.handleAnnouncementsList)
	api.GET("/guideline", s.handleUserGuidelineGet)
	api.POST("/tools/provision/decrypt", s.authSession(), s.handleToolProvisionDecrypt)

	api.POST("/metrics", s.authAgent(), s.handleMetrics)
	api.GET("/node/actions", s.authAgent(), s.handleNodeActions)
	api.GET("/ha/status", s.handleHAStatus)

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
	user.GET("/provision-messages", s.handleUserProvisionMessagesList)
	user.POST("/provision-messages/:id/decrypt-start", s.handleUserProvisionMessageDecryptStart)
	user.POST("/accounts", s.handleUserAccountsUpsert)
	user.PUT("/accounts", s.handleUserAccountsUpdate)
	user.DELETE("/accounts", s.handleUserAccountsDelete)

	// 用户注册/绑定与 SSH 登录校验
	api.GET("/registry/resolve", s.handleRegistryResolve)
	api.GET("/registry/nodes/:node_id/guard-state", s.handleRegistryNodeGuardState)
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
	admin.GET("/users/deleted", s.requireSuperAdmin(), s.handleAdminDeletedUsers)
	admin.GET("/users/duplicates", s.requireSuperAdmin(), s.handleAdminUserDuplicates)
	admin.GET("/users/:username/profile", s.handleAdminUserProfile)
	admin.POST("/users/:username/block", s.requireSuperAdmin(), s.handleAdminUserBlock)
	admin.POST("/users/:username/unblock", s.requireSuperAdmin(), s.handleAdminUserUnblock)
	admin.POST("/users/:username/delete", s.requireSuperAdmin(), s.handleAdminUserDelete)
	admin.POST("/users/:username/archive", s.requireSuperAdmin(), s.handleAdminUserDelete)
	admin.POST("/users/:username/remove", s.requireSuperAdmin(), s.handleAdminUserDelete)
	admin.POST("/users/deleted/:id/restore", s.requireSuperAdmin(), s.handleAdminDeletedUserRestore)
	admin.GET("/users/graduation-due", s.requireSuperAdmin(), s.handleAdminGraduationDueUsers)
	admin.GET("/me/profile", s.handleAdminMyProfileGet)
	admin.PUT("/me/profile", s.handleAdminMyProfileSet)
	admin.POST("/users/graduation-reminders/send", s.requireSuperAdmin(), s.handleAdminGraduationRemindersSend)
	admin.GET("/prices", s.requireSuperAdmin(), s.handleAdminPrices)
	admin.POST("/prices", s.requireSuperAdmin(), s.handleAdminSetPrice)
	admin.GET("/gpu/queue", s.requireSuperAdmin(), s.handleAdminGPUQueue)
	admin.GET("/requests", s.requireReviewPermission(), s.handleAdminRequestsList)
	admin.GET("/registration-requests/overview", s.requireReviewPermission(), s.handleAdminRegistrationRequestsOverview)
	admin.POST("/registration-requests/:id/approve", s.requireReviewPermission(), s.handleAdminRegistrationRequestApprove)
	admin.POST("/registration-requests/:id/reject", s.requireReviewPermission(), s.handleAdminRegistrationRequestReject)
	admin.GET("/register-security/policy", s.requireReviewPermission(), s.handleAdminRegisterSecurityPolicy)
	admin.GET("/register-security/events", s.requireReviewPermission(), s.handleAdminRegisterSecurityEvents)
	admin.GET("/register-security/disposable-domains", s.requireReviewPermission(), s.handleAdminDisposableEmailDomainsList)
	admin.POST("/register-security/disposable-domains", s.requireReviewPermission(), s.handleAdminDisposableEmailDomainsUpsert)
	admin.DELETE("/register-security/disposable-domains/:domain", s.requireReviewPermission(), s.handleAdminDisposableEmailDomainsDelete)
	admin.POST("/requests/:id/approve", s.requireReviewPermission(), s.handleAdminRequestApprove)
	admin.POST("/requests/:id/reject", s.requireReviewPermission(), s.handleAdminRequestReject)
	admin.POST("/requests/:id/reopen", s.requireReviewPermission(), s.handleAdminRequestReopen)
	admin.POST("/requests/batch-review", s.requireReviewPermission(), s.handleAdminRequestsBatchReview)
	admin.GET("/profile-change-requests", s.requireReviewPermission(), s.handleAdminProfileChangeRequestsList)
	admin.POST("/profile-change-requests/:id/approve", s.requireReviewPermission(), s.handleAdminProfileChangeApprove)
	admin.POST("/profile-change-requests/:id/reject", s.requireReviewPermission(), s.handleAdminProfileChangeReject)
	admin.POST("/announcements", s.requireSuperAdmin(), s.handleAnnouncementCreate)
	admin.DELETE("/announcements/:id", s.requireSuperAdmin(), s.handleAnnouncementDelete)
	admin.GET("/notes", s.requireSuperAdmin(), s.handleAdminNotesList)
	admin.POST("/notes", s.requireSuperAdmin(), s.handleAdminNotesCreate)
	admin.PUT("/notes/:id", s.requireSuperAdmin(), s.handleAdminNotesUpdate)
	admin.DELETE("/notes/:id", s.requireSuperAdmin(), s.handleAdminNotesDelete)
	admin.GET("/usage", s.requireSuperAdmin(), s.handleAdminUsage)
	admin.GET("/nodes", s.requireNodesPermission(), s.handleAdminNodes)
	admin.GET("/nodes/:id/detail", s.requireNodesPermission(), s.handleAdminNodeDetail)
	admin.GET("/nodes/:id/security-events", s.requireNodesPermission(), s.handleAdminNodeSecurityEvents)
	admin.POST("/nodes/:id/ssh-guard", s.requireNodesModifyPermission(), s.handleAdminNodeSSHGuardSet)
	admin.GET("/nodes/:id/points-intercept", s.requireNodesPermission(), s.handleAdminNodePointsInterceptGet)
	admin.POST("/nodes/:id/points-intercept", s.requireNodesModifyPermission(), s.handleAdminNodePointsInterceptSet)
	admin.GET("/nodes/:id/disk-quota", s.requireNodesPermission(), s.handleAdminNodeDiskQuotaGet)
	admin.POST("/nodes/:id/disk-quota", s.requireNodesModifyPermission(), s.handleAdminNodeDiskQuotaSet)
	admin.POST("/nodes/:id/disk-quota/apply", s.requireNodesModifyPermission(), s.handleAdminNodeDiskQuotaApply)
	admin.GET("/nodes/:id/price", s.requireNodesPermission(), s.handleAdminNodePriceGet)
	admin.POST("/nodes/:id/price", s.requireNodesModifyPermission(), s.handleAdminNodePriceSet)
	admin.GET("/nodes/:id/ssh-exclusive", s.requireNodesPermission(), s.handleAdminNodeSSHExclusiveGet)
	admin.POST("/nodes/:id/ssh-exclusive", s.requireNodesModifyPermission(), s.handleAdminNodeSSHExclusiveSet)
	admin.GET("/nodes/:id/view-access", s.requireNodesPermission(), s.handleAdminNodeViewAccessGet)
	admin.POST("/nodes/:id/view-access", s.requireNodesModifyPermission(), s.handleAdminNodeViewAccessSet)
	admin.GET("/nodes/:id/ssh-user/:username/platform", s.requireNodesPermission(), s.handleAdminNodeSSHUserPlatform)
	admin.POST("/nodes/:id/sync-now", s.requireNodesModifyPermission(), s.handleAdminNodeSyncNow)
	admin.POST("/nodes/:id/ssh/disconnect-user", s.requireNodesModifyPermission(), s.handleAdminNodeDisconnectSSHUser)
	admin.POST("/nodes/:id/ssh/disconnect-all", s.requireNodesModifyPermission(), s.handleAdminNodeDisconnectAllSSH)
	admin.POST("/nodes/:id/processes/kill-all-users", s.requireNodesModifyPermission(), s.handleAdminNodeKillAllUserProcesses)
	admin.GET("/ha/status", s.requireSuperAdmin(), s.handleAdminHAStatus)
	admin.GET("/usage/export.csv", s.requireSuperAdmin(), s.handleAdminUsageExportCSV)
	admin.GET("/mail/settings", s.requireSuperAdmin(), s.handleAdminMailSettingsGet)
	admin.POST("/mail/settings", s.requireSuperAdmin(), s.handleAdminMailSettingsSet)
	admin.POST("/mail/test", s.requireSuperAdmin(), s.handleAdminMailTest)
	admin.POST("/mail/send", s.requireSuperAdmin(), s.handleAdminMailSend)
	admin.GET("/guideline", s.requireSuperAdmin(), s.handleAdminGuidelineGet)
	admin.POST("/guideline", s.requireSuperAdmin(), s.handleAdminGuidelineSet)
	admin.GET("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsList)
	admin.GET("/accounts/mapping-risks", s.requireSuperAdmin(), s.handleAdminAccountMappingRisks)
	admin.GET("/accounts/provision-logs", s.requireSuperAdmin(), s.handleAdminAccountProvisionLogs)
	admin.POST("/accounts", s.requireSuperAdmin(), s.handleAdminAccountsUpsert)
	admin.POST("/accounts/provision", s.requireSuperAdmin(), s.handleAdminAccountProvision)
	admin.POST("/accounts/disable", s.requireSuperAdmin(), s.handleAdminAccountDisable)
	admin.POST("/accounts/enable", s.requireSuperAdmin(), s.handleAdminAccountEnable)
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
	admin.POST("/power-users/promote", s.requireSuperAdmin(), s.handleAdminPowerUsersPromote)
	admin.PUT("/power-users/:username/permissions", s.requireSuperAdmin(), s.handleAdminPowerUsersUpdatePermissions)
	admin.POST("/power-users/:username/demote", s.requireSuperAdmin(), s.handleAdminPowerUsersDemote)
	admin.DELETE("/power-users/:username", s.requireSuperAdmin(), s.handleAdminPowerUsersDelete)
	admin.GET("/stats/users", s.requireBoardPermission(), s.handleAdminStatsUsers)
	admin.GET("/stats/platform-users", s.requireBoardPermission(), s.handleAdminStatsPlatformUsers)
	admin.GET("/stats/platform-users/:username/nodes", s.requireBoardPermission(), s.handleAdminStatsPlatformUserNodes)
	admin.GET("/stats/monthly", s.requireBoardPermission(), s.handleAdminStatsMonthly)
	admin.GET("/stats/recharges", s.requireBoardPermission(), s.handleAdminStatsRecharges)
	admin.GET("/points/users", s.requireSuperAdmin(), s.handleAdminPointsUsers)
	admin.GET("/points/records", s.requireSuperAdmin(), s.handleAdminPointsRecords)
	admin.POST("/points/adjust", s.requireSuperAdmin(), s.handleAdminPointsAdjust)
	admin.POST("/points/batch-grant", s.requireSuperAdmin(), s.handleAdminPointsBatchGrant)
	admin.POST("/points/batch-adjust-users", s.requireSuperAdmin(), s.handleAdminPointsBatchAdjustUsers)
	admin.GET("/points/special-rules", s.requireSuperAdmin(), s.handleAdminPointsSpecialRulesList)
	admin.POST("/points/special-rules", s.requireSuperAdmin(), s.handleAdminPointsSpecialRulesUpsert)
	admin.DELETE("/points/special-rules/:username", s.requireSuperAdmin(), s.handleAdminPointsSpecialRulesDelete)
	admin.GET("/points/monthly-config", s.requireSuperAdmin(), s.handleAdminPointsMonthlyConfigGet)
	admin.POST("/points/monthly-config", s.requireSuperAdmin(), s.handleAdminPointsMonthlyConfigSet)
	admin.GET("/points/monthly-reset/status", s.requireSuperAdmin(), s.handleAdminPointsMonthlyResetStatus)
	admin.POST("/points/monthly-reset", s.requireSuperAdmin(), s.handleAdminPointsMonthlyReset)

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
		role, perms, ok, err := s.store.ResolveSessionRolePerms(c.Request.Context(), p.Username)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("auth_user", p.Username)
		c.Set("auth_role", role)
		c.Set("auth_perms", perms)
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
			c.Set("auth_perms", uint32(permViewBoard|permViewNodes|permManageNodes|permReviewRequests))
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
		if err != nil || strings.TrimSpace(p.Username) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		role, perms, ok, err := s.store.ResolveSessionRolePerms(c.Request.Context(), p.Username)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok || (role != "admin" && role != "power_user") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("auth_method", "session")
		c.Set("auth_user", p.Username)
		c.Set("auth_role", role)
		c.Set("auth_perms", perms)
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

func (s *Server) requireNodesModifyPermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := getAuthRole(c)
		if role == "admin" {
			c.Next()
			return
		}
		if role == "power_user" && (getAuthPerms(c)&permManageNodes) != 0 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func (s *Server) ensureNodeReadable(c *gin.Context, nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return false
	}
	role := getAuthRole(c)
	if role == "admin" {
		return true
	}
	if role != "power_user" {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	username := strings.TrimSpace(c.GetString("auth_user"))
	if username == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	ok, err := s.store.CanPowerUserViewNode(c.Request.Context(), nodeID, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	return true
}

func (s *Server) visibleNodeIDsForPowerUser(c *gin.Context) ([]string, bool, error) {
	if getAuthRole(c) != "power_user" {
		return nil, false, nil
	}
	username := strings.TrimSpace(c.GetString("auth_user"))
	if username == "" {
		return nil, true, errors.New("forbidden")
	}
	visible, err := s.store.ListVisibleNodeIDsForPowerUser(c.Request.Context(), username)
	if err != nil {
		return nil, true, err
	}
	return visible, true, nil
}

func (s *Server) adminUsernameSet(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.store.ListAdminUsernames(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(rows))
	for _, u := range rows {
		u = strings.TrimSpace(u)
		if u != "" {
			out[u] = struct{}{}
		}
	}
	return out, nil
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
	exclusiveTotal, err := s.store.GetUserExclusiveBalanceTotal(ctx, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	monthlyCfg, err := s.store.GetMonthlyPointsConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	effectiveBalance := u.Balance + u.CarryoverBalance
	effectiveStatus := EffectiveStatusForBalance(u.Status, effectiveBalance, s.cfg.WarningThreshold, s.cfg.LimitedThreshold)
	currentOverdraft := 0.0
	if effectiveBalance < 0 {
		currentOverdraft = -effectiveBalance
	}
	maxOverdraftLimit := normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit)

	c.JSON(http.StatusOK, gin.H{
		"username":                    u.Username,
		"balance":                     u.Balance,
		"general_balance":             u.Balance,
		"carryover_balance":           u.CarryoverBalance,
		"exclusive_balance":           exclusiveTotal,
		"total_balance":               u.Balance + u.CarryoverBalance + exclusiveTotal,
		"status":                      effectiveStatus,
		"warning_threshold_points":    s.cfg.WarningThreshold,
		"limited_threshold_points":    s.cfg.LimitedThreshold,
		"monthly_max_overdraft_limit": maxOverdraftLimit,
		"current_overdraft_points":    currentOverdraft,
		"overdraft_exceeded":          isOverdraftExceeded(effectiveBalance, maxOverdraftLimit),
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
	quotaRefreshTargets := 0
	quotaRefreshError := ""
	if s.cfg.EnableCPUControl {
		n, err := s.refreshCPUQuotaForBillingUser(
			ctx,
			username,
			res.User.Balance+res.User.CarryoverBalance,
			fmt.Sprintf("管理员充值后重算限速（%s）", username),
		)
		if err != nil {
			quotaRefreshError = err.Error()
			log.Printf("充值后刷新 CPU 限速失败 username=%s err=%v", username, err)
		} else {
			quotaRefreshTargets = n
		}
	}
	gpuRefreshTargets := 0
	gpuRefreshError := ""
	memoryRefreshTargets := 0
	memoryRefreshError := ""
	n, err := s.refreshGPUAccessForBillingUser(
		ctx,
		username,
		res.User.Balance+res.User.CarryoverBalance,
		fmt.Sprintf("管理员充值后同步 GPU 限制（%s）", username),
	)
	if err != nil {
		gpuRefreshError = err.Error()
		log.Printf("充值后刷新 GPU 限制失败 username=%s err=%v", username, err)
	} else {
		gpuRefreshTargets = n
	}
	n, err = s.refreshMemoryLimitForBillingUser(
		ctx,
		username,
		res.User.Balance+res.User.CarryoverBalance,
		fmt.Sprintf("管理员充值后同步内存限额（%s）", username),
	)
	if err != nil {
		memoryRefreshError = err.Error()
		log.Printf("充值后刷新内存限额失败 username=%s err=%v", username, err)
	} else {
		memoryRefreshTargets = n
	}

	c.JSON(http.StatusOK, gin.H{
		"username":               res.User.Username,
		"balance":                res.User.Balance,
		"general_balance":        res.User.Balance,
		"carryover_balance":      res.User.CarryoverBalance,
		"exclusive_balance":      0,
		"total_balance":          res.User.Balance + res.User.CarryoverBalance,
		"status":                 res.User.Status,
		"quota_refresh_targets":  quotaRefreshTargets,
		"quota_refresh_error":    quotaRefreshError,
		"gpu_refresh_targets":    gpuRefreshTargets,
		"gpu_refresh_error":      gpuRefreshError,
		"memory_refresh_targets": memoryRefreshTargets,
		"memory_refresh_error":   memoryRefreshError,
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

func (s *Server) handleAdminUserProfile(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	ctx := c.Request.Context()
	acc, err := s.store.GetUserAccountByUsername(ctx, username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	role := "user"
	email := ""
	realName := ""
	studentID := ""
	advisor := ""
	expectedGradYear := 0
	expectedGradMonth := 0
	phone := ""
	var createdAt any = nil
	var updatedAt any = nil

	if err == nil {
		role = acc.Role
		email = acc.Email
		realName = acc.RealName
		studentID = acc.StudentID
		advisor = acc.Advisor
		expectedGradYear = acc.ExpectedGraduationYear
		expectedGradMonth = acc.ExpectedGraduationMonth
		phone = acc.Phone
		createdAt = acc.CreatedAt
		updatedAt = acc.UpdatedAt
	} else {
		// 兼容 admin/power_user：这些账号不在 user_accounts 里，也需要可点击查看详情。
		rows, detailErr := s.store.ListAdminUserDetails(ctx, 5000)
		if detailErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": detailErr.Error()})
			return
		}
		found := false
		for _, d := range rows {
			if strings.TrimSpace(d.Username) != username {
				continue
			}
			found = true
			role = d.Role
			email = d.Email
			realName = d.RealName
			studentID = d.StudentID
			advisor = d.Advisor
			expectedGradYear = d.ExpectedGradYear
			expectedGradMonth = d.ExpectedGradMonth
			phone = d.Phone
			break
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "平台账号不存在"})
			return
		}
		if getAuthRole(c) == "power_user" && role == "admin" {
			c.JSON(http.StatusNotFound, gin.H{"error": "平台账号不存在"})
			return
		}
		if role == "admin" || role == "power_user" {
			p, pErr := s.store.GetAdminProfile(ctx, username)
			if pErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
				return
			}
			realName = strings.TrimSpace(p.RealName)
			email = strings.TrimSpace(p.Email)
			phone = strings.TrimSpace(p.Phone)
			createdAt = p.CreatedAt
			updatedAt = p.UpdatedAt
		}
	}

	u, err := s.store.GetUser(ctx, username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	balance := s.cfg.DefaultBalance
	carryoverBalance := 0.0
	status := "normal"
	if err == nil {
		balance = u.Balance
		carryoverBalance = u.CarryoverBalance
		status = u.Status
	}
	exclusiveTotal, err := s.store.GetUserExclusiveBalanceTotal(ctx, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exclusiveRows, err := s.store.ListNodeExclusivePointsByUser(ctx, username, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accounts, err := s.store.ListUserNodeAccountsByBilling(ctx, username, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if getAuthRole(c) == "power_user" {
		viewer := strings.TrimSpace(c.GetString("auth_user"))
		if viewer != "" {
			visible, vErr := s.store.ListVisibleNodeIDsForPowerUser(ctx, viewer)
			if vErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": vErr.Error()})
				return
			}
			allow := map[string]struct{}{}
			for _, id := range visible {
				allow[strings.TrimSpace(id)] = struct{}{}
			}
			filtered := make([]UserNodeAccount, 0, len(accounts))
			for _, a := range accounts {
				if _, ok := allow[strings.TrimSpace(a.NodeID)]; ok {
					filtered = append(filtered, a)
				}
			}
			accounts = filtered
			filteredExclusive := make([]NodeExclusivePointsBalance, 0, len(exclusiveRows))
			exclusiveTotal = 0
			for _, x := range exclusiveRows {
				if _, ok := allow[strings.TrimSpace(x.NodeID)]; !ok {
					continue
				}
				filteredExclusive = append(filteredExclusive, x)
				exclusiveTotal += x.Balance
			}
			exclusiveRows = filteredExclusive
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"username":                  username,
			"email":                     email,
			"real_name":                 realName,
			"student_id":                studentID,
			"advisor":                   advisor,
			"expected_graduation_year":  expectedGradYear,
			"expected_graduation_month": expectedGradMonth,
			"phone":                     phone,
			"role":                      role,
			"balance":                   balance,
			"general_balance":           balance,
			"carryover_balance":         carryoverBalance,
			"exclusive_balance":         exclusiveTotal,
			"total_balance":             balance + carryoverBalance + exclusiveTotal,
			"exclusive_balances":        exclusiveRows,
			"status":                    status,
			"created_at":                createdAt,
			"updated_at":                updatedAt,
			"node_accounts":             accounts,
		},
	})
}

type adminMyProfileSetReq struct {
	RealName string `json:"real_name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

func (s *Server) handleAdminMyProfileGet(c *gin.Context) {
	username := strings.TrimSpace(c.GetString("auth_user"))
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	p, err := s.store.GetAdminProfile(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": p})
}

func (s *Server) handleAdminMyProfileSet(c *gin.Context) {
	username := strings.TrimSpace(c.GetString("auth_user"))
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req adminMyProfileSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	p, err := s.store.UpsertAdminProfile(c.Request.Context(), username, req.RealName, req.Email, req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "profile": p})
}

type adminUserDeleteReq struct {
	Reason string `json:"reason"`
}

type adminUserBlockReq struct {
	Reason string `json:"reason"`
}

func (s *Server) handleAdminUserBlock(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	var req adminUserBlockReq
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)
	operator := s.currentOperator(c)
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		if _, err := s.store.SetUserManualStatusTx(c.Request.Context(), tx, username, "blocked"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entries := make([]sshListEntry, 0, 64)
	entrySeen := map[string]struct{}{}
	addEntry := func(nodeID string, localUsername string) {
		nodeID = strings.TrimSpace(nodeID)
		localUsername = strings.TrimSpace(localUsername)
		if nodeID == "" || localUsername == "" {
			return
		}
		key := nodeID + "|" + localUsername
		if _, ok := entrySeen[key]; ok {
			return
		}
		entrySeen[key] = struct{}{}
		entries = append(entries, sshListEntry{
			NodeID:                 nodeID,
			LocalUsername:          localUsername,
			SourceType:             "platform",
			SourcePlatformUsername: username,
		})
	}
	// 平台账号拉黑统一写入全局(*)黑名单，避免同名节点账号在名单中重复展示。
	accounts, err := s.store.ListUserNodeAccountsByBilling(c.Request.Context(), username, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accountTargets := make([]UserNodeAccount, 0, len(accounts))
	mappedLocalSet := map[string]struct{}{}
	for _, acc := range accounts {
		accNode := strings.TrimSpace(acc.NodeID)
		accLocal := strings.TrimSpace(acc.LocalUsername)
		if accNode == "" || accLocal == "" {
			continue
		}
		addEntry("*", accLocal)
		mappedLocalSet[accLocal] = struct{}{}
		accountTargets = append(accountTargets, UserNodeAccount{
			NodeID:        accNode,
			LocalUsername: accLocal,
		})
	}
	// 有映射时仅写入真实节点账号，避免额外生成“平台账号同名”的冗余黑名单记录。
	// 无映射时再回退到平台账号名本身，确保仍可阻断同名本地账号登录。
	if len(accountTargets) == 0 {
		addEntry("*", username)
	} else if _, ok := mappedLocalSet[username]; !ok {
		_ = s.store.DeleteBlacklistExact(c.Request.Context(), "*", username)
		_ = s.store.DeleteSSHListSource(c.Request.Context(), "blacklist", "*", username)
	}
	if _, err := s.upsertBlacklistEntries(c.Request.Context(), entries, operator, reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	forceSyncNodes := map[string]struct{}{}
	if nodes, err := s.store.ListNodes(c.Request.Context(), 5000); err == nil {
		for _, n := range nodes {
			id := strings.TrimSpace(n.NodeID)
			if id != "" {
				forceSyncNodes[id] = struct{}{}
			}
		}
	}
	for _, e := range entries {
		s.enqueueKickSSHUser(c.Request.Context(), e.NodeID, e.LocalUsername, fmt.Sprintf("管理员 %s 拉黑平台账号，已强制断开 SSH 会话", operator))
	}
	for _, acc := range accountTargets {
		s.enqueueNodeAction(acc.NodeID, Action{
			Type:     "kill_all_processes",
			Username: acc.LocalUsername,
			Reason:   fmt.Sprintf("管理员 %s 拉黑平台账号 %s：立即终止该账号全部进程", operator, username),
		})
		forceSyncNodes[acc.NodeID] = struct{}{}
	}
	for nodeID := range forceSyncNodes {
		s.enqueueNodeAction(nodeID, Action{
			Type:   "force_sync",
			Reason: fmt.Sprintf("管理员 %s 拉黑平台账号 %s：立即刷新 SSH 黑名单策略", operator, username),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                 true,
		"username":           username,
		"status":             "blocked",
		"ssh_blacklisted":    true,
		"covered_accounts":   len(entries),
		"covered_node_count": len(forceSyncNodes),
	})
}

func (s *Server) handleAdminUserUnblock(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		_, err := s.store.SetUserManualStatusTx(c.Request.Context(), tx, username, "normal")
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 清理该账号对应的黑名单条目（包含 * 与具体节点）：
	// 1) source_platform_username == 平台账号（按平台账号添加时记录）
	// 2) billing_username == 平台账号（由映射推断）
	rows, err := s.store.ListBlacklist(c.Request.Context(), "", 200000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.fillSSHEntryDisplayMeta(c.Request.Context(), "", "blacklist", nil, rows, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targets := map[string]struct{}{}
	addTarget := func(nodeID string, localUsername string) {
		nodeID = strings.TrimSpace(nodeID)
		localUsername = strings.TrimSpace(localUsername)
		if nodeID == "" || localUsername == "" {
			return
		}
		targets[nodeID+"|"+localUsername] = struct{}{}
	}
	for _, r := range rows {
		billing := strings.TrimSpace(r.BillingUsername)
		sourcePlatform := strings.TrimSpace(r.SourcePlatformUsername)
		if billing != username && sourcePlatform != username {
			continue
		}
		addTarget(r.NodeID, r.LocalUsername)
	}
	for k := range targets {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 {
			continue
		}
		nodeID := parts[0]
		local := parts[1]
		_ = s.store.DeleteBlacklistExact(c.Request.Context(), nodeID, local)
		_ = s.store.DeleteSSHListSource(c.Request.Context(), "blacklist", nodeID, local)
	}
	forceSyncNodes := map[string]struct{}{}
	for k := range targets {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 {
			continue
		}
		nodeID := strings.TrimSpace(parts[0])
		if nodeID == "" {
			continue
		}
		if nodeID == "*" {
			nodes, err := s.store.ListNodes(c.Request.Context(), 5000)
			if err != nil {
				continue
			}
			for _, n := range nodes {
				id := strings.TrimSpace(n.NodeID)
				if id != "" {
					forceSyncNodes[id] = struct{}{}
				}
			}
			continue
		}
		forceSyncNodes[nodeID] = struct{}{}
	}
	operator := s.currentOperator(c)
	for nodeID := range forceSyncNodes {
		s.enqueueNodeAction(nodeID, Action{
			Type:   "force_sync",
			Reason: fmt.Sprintf("管理员 %s 解除平台账号 %s 黑名单：立即刷新 SSH 策略缓存", operator, username),
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "username": username, "status": "normal", "ssh_unblacklisted": true})
}

func (s *Server) handleAdminUserDelete(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	var req adminUserDeleteReq
	_ = c.ShouldBindJSON(&req)
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	var deleted DeletedUserAccount
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var txErr error
		deleted, txErr = s.store.ArchiveUserAccountTx(c.Request.Context(), tx, username, operator, req.Reason)
		return txErr
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "平台账号不存在或已删除"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
}

func (s *Server) handleAdminDeletedUsers(c *gin.Context) {
	includeRestored := strings.TrimSpace(c.Query("include_restored")) == "1"
	limit := parseLimit(c.Query("limit"), 1000, 5000)
	rows, err := s.store.ListDeletedUserAccounts(c.Request.Context(), includeRestored, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": rows})
}

func (s *Server) handleAdminDeletedUserRestore(c *gin.Context) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	var restored DeletedUserAccount
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var txErr error
		restored, txErr = s.store.RestoreDeletedUserAccountTx(c.Request.Context(), tx, id, operator)
		return txErr
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "restored": restored})
}

func (s *Server) handleAdminUserDuplicates(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	email := strings.TrimSpace(strings.ToLower(c.Query("email")))
	studentID := strings.TrimSpace(c.Query("student_id"))
	limit := parseLimit(c.Query("limit"), 200, 1000)
	active, deleted, err := s.store.FindPlatformUsersByIdentity(c.Request.Context(), username, email, studentID, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"active_users": active, "deleted_users": deleted})
}

func (s *Server) handleAdminGraduationDueUsers(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 2000, 5000)
	rows, err := s.store.ListGraduationDueUsers(c.Request.Context(), time.Now(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": rows})
}

type graduationRemindersSendReq struct {
	Usernames []string `json:"usernames"`
}

type graduationReminderSendResult struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

func (s *Server) handleAdminGraduationRemindersSend(c *gin.Context) {
	var req graduationRemindersSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Usernames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usernames 不能为空"})
		return
	}
	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	monthStart := time.Now().Format("2006-01")
	results := make([]graduationReminderSendResult, 0, len(req.Usernames))
	okCount := 0
	for _, raw := range req.Usernames {
		username := strings.TrimSpace(raw)
		if username == "" {
			continue
		}
		r := graduationReminderSendResult{Username: username}
		email, err := s.store.GetUserEmailByUsername(c.Request.Context(), username)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				r.Success = false
				r.Error = "未找到该用户邮箱"
				results = append(results, r)
				continue
			}
			r.Success = false
			r.Error = err.Error()
			results = append(results, r)
			continue
		}
		r.Email = email
		subject := "GPU Ops 数据备份提醒（毕业到期）"
		body := fmt.Sprintf(
			"你好 %s，\n\n系统检测到你已达到预计毕业时间（%s）。\n请尽快备份个人数据，平台将在两个月内进行数据清理。\n如需延期，请尽快联系管理员。\n\nGPU Ops 团队",
			username,
			monthStart,
		)
		if err := sendPlainTextMail(settings, email, subject, body); err != nil {
			r.Success = false
			r.Error = "发送失败: " + err.Error()
			results = append(results, r)
			continue
		}
		r.Success = true
		okCount++
		results = append(results, r)
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"total":   len(results),
		"success": okCount,
		"failed":  len(results) - okCount,
		"results": results,
	})
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

func (s *Server) handleUserGuidelineGet(c *gin.Context) {
	content, updatedBy, updatedAt, err := s.store.GetUserGuideline(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"content":    content,
		"updated_by": updatedBy,
		"updated_at": updatedAt,
	})
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

type adminGuidelineSetReq struct {
	Content string `json:"content"`
}

type adminNoteUpsertReq struct {
	NoteDate string `json:"note_date"` // YYYY-MM-DD
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func (s *Server) handleAdminGuidelineGet(c *gin.Context) {
	s.handleUserGuidelineGet(c)
}

func (s *Server) handleAdminGuidelineSet(c *gin.Context) {
	var req adminGuidelineSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertUserGuideline(c.Request.Context(), req.Content, s.currentOperator(c)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, updatedBy, updatedAt, err := s.store.GetUserGuideline(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"content":    content,
		"updated_by": updatedBy,
		"updated_at": updatedAt,
	})
}

func parseDateYYYYMMDD(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, errors.New("note_date 不能为空")
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, errors.New("note_date 格式应为 YYYY-MM-DD")
	}
	return t, nil
}

func (s *Server) handleAdminNotesList(c *gin.Context) {
	from := strings.TrimSpace(c.Query("from"))
	to := strings.TrimSpace(c.Query("to"))
	limit := parseLimit(c.Query("limit"), 500, 5000)
	rows, err := s.store.ListAdminNotes(c.Request.Context(), from, to, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notes": rows})
}

func (s *Server) handleAdminNotesCreate(c *gin.Context) {
	var req adminNoteUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	noteDate, err := parseDateYYYYMMDD(req.NoteDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := s.currentOperator(c)
	n, err := s.store.CreateAdminNote(c.Request.Context(), noteDate, req.Title, req.Content, operator)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": n})
}

func (s *Server) handleAdminNotesUpdate(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	var req adminNoteUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	noteDate, err := parseDateYYYYMMDD(req.NoteDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := s.currentOperator(c)
	n, err := s.store.UpdateAdminNote(c.Request.Context(), id, noteDate, req.Title, req.Content, operator)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": n})
}

func (s *Server) handleAdminNotesDelete(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	if err := s.store.DeleteAdminNote(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
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
	if role == "admin" {
		c.JSON(http.StatusOK, gin.H{
			"username": username,
			"role":     role,
		})
		return
	}
	acc, err := s.store.GetUserAccountByUsername(c.Request.Context(), username)
	if err != nil {
		if role == "power_user" && errors.Is(err, sql.ErrNoRows) {
			// 兼容“仅创建在 power_users 表中的高级用户”，保留最小可用信息，避免前端空白报错。
			c.JSON(http.StatusOK, gin.H{
				"username": username,
				"role":     role,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	acc.Role = role
	c.JSON(http.StatusOK, acc)
}

func (s *Server) handleUserMyBalance(c *gin.Context) {
	username := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	balance := s.cfg.DefaultBalance
	carryoverBalance := 0.0
	status := "normal"
	u, err := s.store.GetUser(c.Request.Context(), username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err == nil {
		balance = u.Balance
		carryoverBalance = u.CarryoverBalance
		status = u.Status
	}
	exclusiveTotal, err := s.store.GetUserExclusiveBalanceTotal(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exclusiveRows, err := s.store.ListNodeExclusivePointsByUser(c.Request.Context(), username, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	accountCreatedAt, err := s.store.GetUserAccountCreatedAt(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	monthUsedPoints, totalUsedPoints, err := s.store.GetUserUsageCostSummary(c.Request.Context(), username, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	monthlyCfg, err := s.store.GetMonthlyPointsConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	effectiveBalance := balance + carryoverBalance
	effectiveStatus := EffectiveStatusForBalance(status, effectiveBalance, s.cfg.WarningThreshold, s.cfg.LimitedThreshold)
	currentOverdraft := 0.0
	if effectiveBalance < 0 {
		currentOverdraft = -effectiveBalance
	}
	maxOverdraftLimit := normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit)
	totalBalance := balance + carryoverBalance + exclusiveTotal
	c.JSON(http.StatusOK, gin.H{
		"username":                    username,
		"balance":                     balance,
		"general_balance":             balance,
		"carryover_balance":           carryoverBalance,
		"exclusive_balance":           exclusiveTotal,
		"total_balance":               totalBalance,
		"exclusive_balances":          exclusiveRows,
		"status":                      effectiveStatus,
		"account_created_at":          accountCreatedAt,
		"month_remaining_points":      totalBalance,
		"month_used_points":           monthUsedPoints,
		"total_used_points":           totalUsedPoints,
		"warning_threshold_points":    s.cfg.WarningThreshold,
		"limited_threshold_points":    s.cfg.LimitedThreshold,
		"monthly_max_overdraft_limit": maxOverdraftLimit,
		"current_overdraft_points":    currentOverdraft,
		"overdraft_exceeded":          isOverdraftExceeded(effectiveBalance, maxOverdraftLimit),
	})
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
	Email                   string `json:"email"`
	Username                string `json:"username"`
	StudentID               string `json:"student_id"`
	RealName                string `json:"real_name"`
	Advisor                 string `json:"advisor"`
	ExpectedGraduationYear  int    `json:"expected_graduation_year"`
	ExpectedGraduationMonth int    `json:"expected_graduation_month"`
	Phone                   string `json:"phone"`
	ChangeReason            string `json:"change_reason"`
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
	if err := s.store.UpdateUserProfileBase(c.Request.Context(), authUsername, req.RealName, req.Advisor, req.ExpectedGraduationYear, req.ExpectedGraduationMonth, req.Phone); err != nil {
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

func (s *Server) handleUserProvisionMessagesList(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	limit := parseLimit(c.Query("limit"), 100, 1000)
	rows, err := s.store.ListUserProvisionMessages(c.Request.Context(), billing, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": rows})
}

func (s *Server) handleUserProvisionMessageDecryptStart(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message_id 不合法"})
		return
	}
	msg, err := s.store.MarkUserProvisionMessageDecryptStarted(c.Request.Context(), billing, id, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			c.JSON(http.StatusNotFound, gin.H{"error": "密钥通知不存在"})
			return
		case errors.Is(err, errProvisionMessageDestroyed):
			c.JSON(http.StatusGone, gin.H{"error": "该密钥通知已销毁，请联系管理员重新下发"})
			return
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": msg,
	})
}

func (s *Server) handleUserAccountsUpsert(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req userAccountUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	if req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
		return
	}
	reserved, reservedMsg, err := s.validateAdminReservedNodeMapping(c.Request.Context(), req.NodeID, req.LocalUsername, billing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reserved {
		c.JSON(http.StatusForbidden, gin.H{
			"error":          reservedMsg,
			"reason":         "admin_local_username_reserved",
			"node_id":        req.NodeID,
			"local_username": req.LocalUsername,
		})
		return
	}
	mappedBilling, mapped, err := s.store.ResolveBillingUsername(c.Request.Context(), req.NodeID, req.LocalUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mapped && strings.TrimSpace(mappedBilling) != "" && strings.TrimSpace(mappedBilling) != billing {
		c.JSON(http.StatusConflict, gin.H{
			"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s。若冒充绑定他人账号将按平台规则追责，请更换节点账号或联系管理员核验。", req.NodeID, req.LocalUsername, mappedBilling),
			"reason":                  "mapping_exists_other_user",
			"node_id":                 req.NodeID,
			"local_username":          req.LocalUsername,
			"mapped_billing_username": mappedBilling,
		})
		return
	}
	if err := s.store.UpsertUserNodeAccountWithAudit(
		c.Request.Context(),
		req.NodeID,
		req.LocalUsername,
		billing,
		billing,
		"user",
		"用户新增/更新节点账号映射",
	); err != nil {
		var conflictErr *NodeAccountOwnershipConflictError
		if errors.As(err, &conflictErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s。严禁冒充绑定，冒充行为将追责，请联系管理员处理。", conflictErr.NodeID, conflictErr.LocalUsername, conflictErr.ExistingBilling),
				"reason":                  "mapping_exists_other_user",
				"node_id":                 conflictErr.NodeID,
				"local_username":          conflictErr.LocalUsername,
				"mapped_billing_username": conflictErr.ExistingBilling,
			})
			return
		}
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
	req.OldNodeID = strings.TrimSpace(req.OldNodeID)
	req.OldLocalUsername = strings.TrimSpace(req.OldLocalUsername)
	req.NewNodeID = strings.TrimSpace(req.NewNodeID)
	req.NewLocalUsername = strings.TrimSpace(req.NewLocalUsername)
	if req.OldNodeID == "" || req.OldLocalUsername == "" || req.NewNodeID == "" || req.NewLocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "节点编号和节点账号不能为空"})
		return
	}
	reserved, reservedMsg, err := s.validateAdminReservedNodeMapping(c.Request.Context(), req.NewNodeID, req.NewLocalUsername, billing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reserved {
		c.JSON(http.StatusForbidden, gin.H{
			"error":          reservedMsg,
			"reason":         "admin_local_username_reserved",
			"node_id":        req.NewNodeID,
			"local_username": req.NewLocalUsername,
		})
		return
	}
	targetBilling, targetMapped, err := s.store.ResolveBillingUsername(c.Request.Context(), req.NewNodeID, req.NewLocalUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if targetMapped && strings.TrimSpace(targetBilling) != "" && strings.TrimSpace(targetBilling) != billing {
		c.JSON(http.StatusConflict, gin.H{
			"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s。若冒充绑定他人账号将按平台规则追责，请更换节点账号或联系管理员核验。", req.NewNodeID, req.NewLocalUsername, targetBilling),
			"reason":                  "mapping_exists_other_user",
			"node_id":                 req.NewNodeID,
			"local_username":          req.NewLocalUsername,
			"mapped_billing_username": targetBilling,
		})
		return
	}
	if err := s.store.UpdateUserNodeAccountWithAudit(
		c.Request.Context(),
		req.OldNodeID, req.OldLocalUsername, billing,
		req.NewNodeID, req.NewLocalUsername, billing,
		billing, "user", "用户修改节点账号映射",
	); err != nil {
		var conflictErr *NodeAccountOwnershipConflictError
		if errors.As(err, &conflictErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s。严禁冒充绑定，冒充行为将追责，请联系管理员处理。", conflictErr.NodeID, conflictErr.LocalUsername, conflictErr.ExistingBilling),
				"reason":                  "mapping_exists_other_user",
				"node_id":                 conflictErr.NodeID,
				"local_username":          conflictErr.LocalUsername,
				"mapped_billing_username": conflictErr.ExistingBilling,
			})
			return
		}
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
	if err := s.store.DeleteUserNodeAccountWithAudit(
		c.Request.Context(),
		nodeID, localUsername, billing,
		billing, "user", "用户删除节点账号映射",
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 删除映射后立即收敛该本地账号会话，避免“映射已删但进程/SSH仍在运行”。
	s.enqueueNodeAction(nodeID, Action{
		Type:     "kill_all_processes",
		Username: localUsername,
		Reason:   fmt.Sprintf("平台用户 %s 删除节点账号映射：强制终止该账号全部进程", billing),
	})
	s.enqueueNodeAction(nodeID, Action{
		Type:     "kick_ssh_user",
		Username: localUsername,
		Reason:   fmt.Sprintf("平台用户 %s 删除节点账号映射：强制断开 SSH 会话", billing),
	})
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("平台用户 %s 删除节点账号映射：立即刷新节点 SSH 策略缓存", billing),
	})
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

type adminAccountProvisionReq struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
	SSHHost         string `json:"ssh_host"`
	SSHPort         int    `json:"ssh_port"`
	RotateKey       bool   `json:"rotate_key"`
}

type toolProvisionDecryptReq struct {
	EncryptedPayload string `json:"encrypted_payload"`
	DecryptCode      string `json:"decrypt_code"`
}

type adminAccountDisableReq struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
	Reason          string `json:"reason"`
}

type adminAccountEnableReq struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
	Reason          string `json:"reason"`
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

func (s *Server) handleAdminAccountProvisionLogs(c *gin.Context) {
	billing := strings.TrimSpace(c.Query("billing_username"))
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.store.ListAccountProvisionLogs(c.Request.Context(), billing, nodeID, localUsername, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": rows})
}

func (s *Server) handleAdminAccountMappingRisks(c *gin.Context) {
	days := parseLimit(c.Query("days"), 30, 365)
	minSwitches := parseLimit(c.Query("min_switches"), 2, 100)
	limit := parseLimit(c.Query("limit"), 300, 5000)
	rows, err := s.store.ListUserNodeAccountMappingRisks(c.Request.Context(), days, minSwitches, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"days":           days,
		"min_switches":   minSwitches,
		"total_risky":    len(rows),
		"risky_accounts": rows,
	})
}

func (s *Server) handleAdminAccountsUpsert(c *gin.Context) {
	var req adminAccountUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	reserved, reservedMsg, err := s.validateAdminReservedNodeMapping(c.Request.Context(), req.NodeID, req.LocalUsername, req.BillingUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reserved {
		c.JSON(http.StatusForbidden, gin.H{
			"error":            reservedMsg,
			"reason":           "admin_local_username_reserved",
			"billing_username": req.BillingUsername,
			"node_id":          req.NodeID,
			"local_username":   req.LocalUsername,
		})
		return
	}
	operator := s.currentOperator(c)
	if err := s.store.UpsertUserNodeAccountWithAudit(
		c.Request.Context(),
		req.NodeID,
		req.LocalUsername,
		req.BillingUsername,
		operator,
		"admin",
		"管理员新增/更新节点账号映射",
	); err != nil {
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
	req.OldBillingUsername = strings.TrimSpace(req.OldBillingUsername)
	req.OldNodeID = strings.TrimSpace(req.OldNodeID)
	req.OldLocalUsername = strings.TrimSpace(req.OldLocalUsername)
	req.NewBillingUsername = strings.TrimSpace(req.NewBillingUsername)
	req.NewNodeID = strings.TrimSpace(req.NewNodeID)
	req.NewLocalUsername = strings.TrimSpace(req.NewLocalUsername)
	reserved, reservedMsg, err := s.validateAdminReservedNodeMapping(c.Request.Context(), req.NewNodeID, req.NewLocalUsername, req.NewBillingUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reserved {
		c.JSON(http.StatusForbidden, gin.H{
			"error":            reservedMsg,
			"reason":           "admin_local_username_reserved",
			"billing_username": req.NewBillingUsername,
			"node_id":          req.NewNodeID,
			"local_username":   req.NewLocalUsername,
		})
		return
	}
	operator := s.currentOperator(c)
	if err := s.store.UpdateUserNodeAccountWithAudit(
		c.Request.Context(),
		req.OldNodeID, req.OldLocalUsername, req.OldBillingUsername,
		req.NewNodeID, req.NewLocalUsername, req.NewBillingUsername,
		operator, "admin", "管理员修改节点账号映射",
	); err != nil {
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
	operator := s.currentOperator(c)
	if err := s.store.DeleteUserNodeAccountWithAudit(
		c.Request.Context(),
		nodeID, localUsername, billing,
		operator, "admin", "管理员删除节点账号映射",
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:     "kill_all_processes",
		Username: localUsername,
		Reason:   fmt.Sprintf("管理员 %s 删除节点账号映射：强制终止该账号全部进程", operator),
	})
	s.enqueueNodeAction(nodeID, Action{
		Type:     "kick_ssh_user",
		Username: localUsername,
		Reason:   fmt.Sprintf("管理员 %s 删除节点账号映射：强制断开 SSH 会话", operator),
	})
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 删除节点账号映射：立即刷新节点 SSH 策略缓存", operator),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminAccountDisable(c *gin.Context) {
	var req adminAccountDisableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
		return
	}

	mappedBilling, mapped, err := s.store.ResolveBillingUsername(c.Request.Context(), req.NodeID, req.LocalUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !mapped {
		c.JSON(http.StatusNotFound, gin.H{"error": "该节点账号映射不存在"})
		return
	}
	if req.BillingUsername != "" && req.BillingUsername != mappedBilling {
		c.JSON(http.StatusBadRequest, gin.H{"error": "映射校验失败：该节点账号不属于指定平台账号"})
		return
	}

	operator := s.currentOperator(c)
	reason := req.Reason
	if reason == "" {
		reason = fmt.Sprintf("管理员 %s 禁用了该节点账号映射", operator)
	}
	entries := []sshListEntry{{
		NodeID:                 req.NodeID,
		LocalUsername:          req.LocalUsername,
		SourceType:             "platform",
		SourcePlatformUsername: mappedBilling,
	}}
	result, err := s.upsertBlacklistEntries(c.Request.Context(), entries, operator, reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.enqueueNodeAction(req.NodeID, Action{
		Type:     "kill_all_processes",
		Username: req.LocalUsername,
		Reason:   fmt.Sprintf("管理员 %s 禁用映射：强制终止该账号所有进程", operator),
	})
	s.enqueueNodeAction(req.NodeID, Action{
		Type:     "kick_ssh_user",
		Username: req.LocalUsername,
		Reason:   fmt.Sprintf("管理员 %s 禁用映射：强制断开 SSH 会话", operator),
	})
	s.enqueueNodeAction(req.NodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 禁用映射，立即刷新节点 SSH 策略缓存", operator),
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":                        true,
		"billing_username":          mappedBilling,
		"node_id":                   req.NodeID,
		"local_username":            req.LocalUsername,
		"blacklisted":               true,
		"killed_processes":          true,
		"kicked_ssh":                true,
		"removed_from_whitelist":    result.RemovedFromWhitelist,
		"removed_from_exemptions":   result.RemovedFromExemptions,
		"message":                   "已禁用该节点账号映射，并下发强制下线与进程终止指令",
		"blacklist_reason_recorded": reason,
	})
}

func (s *Server) handleAdminAccountEnable(c *gin.Context) {
	var req adminAccountEnableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
		return
	}

	mappedBilling, mapped, err := s.store.ResolveBillingUsername(c.Request.Context(), req.NodeID, req.LocalUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !mapped {
		c.JSON(http.StatusNotFound, gin.H{"error": "该节点账号映射不存在"})
		return
	}
	if req.BillingUsername != "" && req.BillingUsername != mappedBilling {
		c.JSON(http.StatusBadRequest, gin.H{"error": "映射校验失败：该节点账号不属于指定平台账号"})
		return
	}

	rows, err := s.store.ListBlacklist(c.Request.Context(), req.NodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exactBlacklisted := false
	for _, r := range rows {
		if strings.TrimSpace(r.NodeID) == req.NodeID && strings.TrimSpace(r.LocalUsername) == req.LocalUsername {
			exactBlacklisted = true
			break
		}
	}
	if !exactBlacklisted {
		c.JSON(http.StatusOK, gin.H{
			"ok":               true,
			"billing_username": mappedBilling,
			"node_id":          req.NodeID,
			"local_username":   req.LocalUsername,
			"unblacklisted":    false,
			"message":          "该映射当前未被禁用",
		})
		return
	}

	_ = s.store.DeleteBlacklistExact(c.Request.Context(), req.NodeID, req.LocalUsername)
	_ = s.store.DeleteSSHListSource(c.Request.Context(), "blacklist", req.NodeID, req.LocalUsername)

	operator := s.currentOperator(c)
	reason := req.Reason
	if reason == "" {
		reason = fmt.Sprintf("管理员 %s 解除禁用该节点账号映射", operator)
	}
	s.enqueueNodeAction(req.NodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("%s：立即刷新节点 SSH 策略缓存", reason),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"billing_username": mappedBilling,
		"node_id":          req.NodeID,
		"local_username":   req.LocalUsername,
		"unblacklisted":    true,
		"message":          "已解除禁用并刷新节点 SSH 策略缓存",
	})
}

func (s *Server) handleAdminAccountProvision(c *gin.Context) {
	var req adminAccountProvisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	req.SSHHost = strings.TrimSpace(req.SSHHost)
	if req.BillingUsername == "" || req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "平台账号、节点编号、节点账号不能为空"})
		return
	}
	if !provisionLocalUsernamePattern.MatchString(req.LocalUsername) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "节点账号格式不合法：仅支持小写字母/数字/下划线/短横线，且必须以字母或下划线开头"})
		return
	}
	reserved, reservedMsg, err := s.validateAdminReservedNodeMapping(c.Request.Context(), req.NodeID, req.LocalUsername, req.BillingUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reserved {
		c.JSON(http.StatusForbidden, gin.H{
			"error":            reservedMsg,
			"reason":           "admin_local_username_reserved",
			"billing_username": req.BillingUsername,
			"node_id":          req.NodeID,
			"local_username":   req.LocalUsername,
		})
		return
	}

	userAcc, err := s.store.GetUserAccountByUsername(c.Request.Context(), req.BillingUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "平台账号不存在，或该账号不是可接收邮件的普通平台用户"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	userEmail := strings.TrimSpace(userAcc.Email)
	if userEmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该平台账号未配置邮箱，无法发送密钥邮件"})
		return
	}

	node, err := s.store.GetNodeStatus(c.Request.Context(), req.NodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	mappedBilling, mapped, err := s.store.ResolveBillingUsername(c.Request.Context(), req.NodeID, req.LocalUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	wasReissued := false
	if mapped {
		mappedBilling = strings.TrimSpace(mappedBilling)
		if mappedBilling != "" && mappedBilling != req.BillingUsername {
			c.JSON(http.StatusConflict, gin.H{
				"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s，请更换节点账号名或先调整映射", req.NodeID, req.LocalUsername, mappedBilling),
				"reason":                  "mapping_exists_other_user",
				"node_id":                 req.NodeID,
				"local_username":          req.LocalUsername,
				"mapped_billing_username": mappedBilling,
			})
			return
		}
		if !req.RotateKey {
			displayBilling := req.BillingUsername
			if mappedBilling != "" {
				displayBilling = mappedBilling
			}
			c.JSON(http.StatusConflict, gin.H{
				"error":                   fmt.Sprintf("节点 %s 的账号 %s 已绑定到平台账号 %s。若该用户丢失密钥，请确认“重新生成新密钥并重发”。", req.NodeID, req.LocalUsername, displayBilling),
				"reason":                  "mapping_exists_same_user",
				"node_id":                 req.NodeID,
				"local_username":          req.LocalUsername,
				"mapped_billing_username": displayBilling,
			})
			return
		}
		wasReissued = true
	}

	sshPort := inferSSHPort(req.NodeID, req.SSHPort)
	sshHost := strings.TrimSpace(req.SSHHost)
	if sshHost == "" {
		sshHost = strings.TrimSpace(node.NodeIP)
	}
	if sshHost == "" {
		sshHost = "<请向管理员确认节点IP>"
	}

	keyCtx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	privateKey, publicKey, err := generateOpenSSHEd25519KeyPair(keyCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 ed25519 密钥失败: " + err.Error()})
		return
	}

	decryptCode, err := randomDecryptCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥提取码失败: " + err.Error()})
		return
	}
	encryptedPayload, err := buildEncryptedPrivateKeyPayload(
		privateKey,
		decryptCode,
		req.NodeID,
		req.LocalUsername,
		req.BillingUsername,
		sshHost,
		sshPort,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密私钥失败: " + err.Error()})
		return
	}

	operator := s.currentOperator(c)
	upsertReason := "管理员开通节点账号并写入映射"
	if wasReissued {
		upsertReason = "管理员重新生成密钥并刷新节点账号映射"
	}
	if err := s.store.UpsertUserNodeAccountWithAudit(
		c.Request.Context(),
		req.NodeID,
		req.LocalUsername,
		req.BillingUsername,
		operator,
		"admin",
		upsertReason,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.enqueueNodeAction(req.NodeID, Action{
		Type:      "create_local_account",
		Username:  req.LocalUsername,
		PublicKey: publicKey,
		Reason: func() string {
			if wasReissued {
				return fmt.Sprintf("管理员 %s 重新生成节点账号密钥，并刷新平台账号 %s 的 authorized_keys", operator, req.BillingUsername)
			}
			return fmt.Sprintf("管理员 %s 开通节点账号，并分配给平台账号 %s", operator, req.BillingUsername)
		}(),
	})
	if dqPolicy, err := s.store.GetNodeDiskQuotaPolicy(c.Request.Context(), req.NodeID); err == nil && dqPolicy.Enabled {
		mountpoint, softMB, hardMB := resolveDiskQuotaPolicyValues(dqPolicy)
		if mountpoint == "" {
			mountpoint = pickPreferredQuotaMount(node.DiskQuotaMounts)
		}
		if mountpoint != "" && (softMB > 0 || hardMB > 0) {
			s.enqueueNodeAction(req.NodeID, Action{
				Type:                "set_disk_quota",
				Username:            req.LocalUsername,
				DiskQuotaMountpoint: mountpoint,
				DiskQuotaSoftMB:     softMB,
				DiskQuotaHardMB:     hardMB,
				Reason:              fmt.Sprintf("管理员 %s 开通节点账号后自动应用默认磁盘配额", operator),
			})
		}
	}
	s.enqueueNodeAction(req.NodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 开通了节点账号，要求节点立即同步本地用户清单", operator),
	})

	baseURL := inferRequestBaseURL(c)
	decryptPath := "/user/accounts?tool=key-decryptor"
	decryptURL := decryptPath
	if baseURL != "" {
		decryptURL = baseURL + decryptPath
	}
	fileName := fmt.Sprintf("%d_%s.txt", sshPort, req.LocalUsername)
	sshCommand := fmt.Sprintf("ssh -i %s %s@%s -p %d", fileName, req.LocalUsername, sshHost, sshPort)
	noticeErrMsg := ""
	if err := s.store.InsertUserProvisionMessage(
		c.Request.Context(),
		req.BillingUsername,
		req.NodeID,
		req.LocalUsername,
		encryptedPayload,
		decryptPath,
		sshHost,
		sshPort,
		fileName,
		sshCommand,
		userEmail,
		operator,
	); err != nil {
		noticeErrMsg = "平台内密钥通知写入失败: " + err.Error()
	}

	mailSent := false
	mailErrMsg := ""
	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		mailErrMsg = "读取邮件配置失败: " + err.Error()
	} else {
		subject := fmt.Sprintf("【GPU Ops】节点账号已开通：%s@%s", req.LocalUsername, req.NodeID)
		body := strings.Join([]string{
			fmt.Sprintf("你好 %s（%s）：", strings.TrimSpace(userAcc.RealName), req.BillingUsername),
			"",
			"管理员已为你开通计算节点账号。",
			"私钥密文和解密步骤已发送到平台内（我的节点账号映射页面）。",
			"本邮件只发送提取码，用于和平台内密文组合解密。",
			"",
			"节点信息：",
			fmt.Sprintf("- 节点编号：%s", req.NodeID),
			fmt.Sprintf("- 节点账号：%s", req.LocalUsername),
			fmt.Sprintf("- SSH 地址：%s:%d", sshHost, sshPort),
			fmt.Sprintf("- 平台内解密入口：%s", decryptURL),
			"",
			"提取码（请妥善保管，不要外传）：",
			decryptCode,
			"",
			"注意：邮件不包含私钥密文，密文只在平台内展示。",
		}, "\n")
		if err := sendPlainTextMail(settings, userEmail, subject, body); err != nil {
			mailErrMsg = "发送邮件失败: " + err.Error()
		} else {
			mailSent = true
		}
	}
	logErrMsg := ""
	if err := s.store.InsertAccountProvisionLog(
		c.Request.Context(),
		req.BillingUsername,
		req.NodeID,
		req.LocalUsername,
		userEmail,
		sshHost,
		sshPort,
		fileName,
		mailSent,
		mailErrMsg,
		operator,
	); err != nil {
		logErrMsg = "开通历史记录写入失败: " + err.Error()
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":                true,
		"node_id":           req.NodeID,
		"local_username":    req.LocalUsername,
		"billing_username":  req.BillingUsername,
		"reissued_key":      wasReissued,
		"email":             userEmail,
		"ssh_host":          sshHost,
		"ssh_port":          sshPort,
		"download_filename": fileName,
		"ssh_command":       sshCommand,
		"decrypt_url":       decryptURL,
		"decrypt_code":      decryptCode,
		"encrypted_payload": encryptedPayload,
		"mail_sent":         mailSent,
		"mail_error":        mailErrMsg,
		"log_error":         logErrMsg,
		"notice_error":      noticeErrMsg,
		"message": func() string {
			if wasReissued {
				return "新密钥重发指令已下发，节点将在下一个心跳周期内刷新账号公钥"
			}
			return "节点账号开通指令已下发，节点将在下一个心跳周期内创建账号并写入公钥"
		}(),
	})
}

func (s *Server) handleToolProvisionDecrypt(c *gin.Context) {
	var req toolProvisionDecryptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.EncryptedPayload = strings.TrimSpace(req.EncryptedPayload)
	if req.EncryptedPayload == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "encrypted_payload 不能为空"})
		return
	}
	if getAuthRole(c) == "user" {
		billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
		_, err := s.store.MarkUserProvisionMessageDecryptStartedByPayload(c.Request.Context(), billing, req.EncryptedPayload, time.Now())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errProvisionMessageDestroyed) {
				c.JSON(http.StatusGone, gin.H{"error": "该密钥通知不存在或已销毁，请联系管理员重新下发"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	privateKey, env, err := decryptProvisionPrivateKeyPayload(req.EncryptedPayload, req.DecryptCode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":          true,
		"private_key": privateKey,
		"envelope":    env,
	})
}

type sshListUpsertReq struct {
	NodeID           string                   `json:"node_id"`
	Usernames        []string                 `json:"usernames"`
	BillingUsernames []string                 `json:"billing_usernames"`
	PlatformAccounts []sshListPlatformAccount `json:"platform_accounts"`
	Reason           string                   `json:"reason"`
}

type sshListPlatformAccount struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
}

type sshListEntry struct {
	NodeID                 string
	LocalUsername          string
	SourceType             string
	SourcePlatformUsername string
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

func (s *Server) isAdminAccountCached(ctx context.Context, username string, cache map[string]bool) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}
	if cache != nil {
		if v, ok := cache[username]; ok {
			return v, nil
		}
	}
	isAdmin, err := s.store.IsAdminAccount(ctx, username)
	if err != nil {
		return false, err
	}
	if cache != nil {
		cache[username] = isAdmin
	}
	return isAdmin, nil
}

func (s *Server) validateAdminReservedNodeMapping(ctx context.Context, nodeID string, localUsername string, billingUsername string) (bool, string, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return false, "", nil
	}
	adminCache := map[string]bool{}
	targetIsAdmin, err := s.isAdminAccountCached(ctx, billingUsername, adminCache)
	if err != nil {
		return false, "", err
	}
	if targetIsAdmin {
		return false, "", nil
	}
	// 管理员账号同名的节点用户名仅允许管理员绑定，避免保留账号被普通用户占用。
	localIsAdminName, err := s.isAdminAccountCached(ctx, localUsername, adminCache)
	if err != nil {
		return false, "", err
	}
	if localIsAdminName {
		return true, fmt.Sprintf("节点账号 %s 为管理员保留账号名，仅管理员可绑定映射", localUsername), nil
	}
	// 若该节点账号当前映射到管理员，也禁止改绑给非管理员。
	currentBilling, mapped, err := s.store.ResolveBillingUsername(ctx, nodeID, localUsername)
	if err != nil {
		return false, "", err
	}
	if mapped {
		currentBilling = strings.TrimSpace(currentBilling)
		if currentBilling != "" {
			currentIsAdmin, err := s.isAdminAccountCached(ctx, currentBilling, adminCache)
			if err != nil {
				return false, "", err
			}
			if currentIsAdmin {
				return true, fmt.Sprintf("节点 %s 的账号 %s 已保留给管理员账号 %s，非管理员不可绑定", nodeID, localUsername, currentBilling), nil
			}
		}
	}
	return false, "", nil
}

func (s *Server) shouldSkipAdminMappedDiskQuotaTarget(
	ctx context.Context,
	nodeID string,
	local NodeLocalUser,
	adminCache map[string]bool,
) (bool, error) {
	if !local.MappingExists {
		return false, nil
	}
	billing := strings.TrimSpace(local.PlatformUsername)
	if billing == "" {
		resolved, mapped, err := s.store.ResolveBillingUsername(ctx, nodeID, local.LocalUsername)
		if err != nil {
			return false, err
		}
		if !mapped {
			return false, nil
		}
		billing = strings.TrimSpace(resolved)
	}
	if billing == "" {
		return false, nil
	}
	return s.isAdminAccountCached(ctx, billing, adminCache)
}

func (s *Server) resolveSSHListEntries(ctx context.Context, req sshListUpsertReq) ([]sshListEntry, error) {
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	manualUsers := trimUniq(req.Usernames)
	billingUsers := trimUniq(req.BillingUsernames)
	if len(manualUsers) == 0 && len(billingUsers) == 0 && len(req.PlatformAccounts) == 0 {
		return nil, errors.New("usernames/billing_usernames/platform_accounts 至少填写一项")
	}

	entries := map[string]sshListEntry{}
	addEntry := func(node, local string, sourceType string, sourcePlatformUsername string) {
		node = strings.TrimSpace(node)
		local = strings.TrimSpace(local)
		if node == "" || local == "" {
			return
		}
		if sourceType != "platform" {
			sourceType = "local"
			sourcePlatformUsername = ""
		}
		k := node + "|" + local
		if old, ok := entries[k]; ok {
			// 同一条目若同时命中“节点账号添加”和“平台账号添加”，优先记录为平台账号来源。
			if old.SourceType == "platform" {
				return
			}
		}
		entries[k] = sshListEntry{
			NodeID:                 node,
			LocalUsername:          local,
			SourceType:             sourceType,
			SourcePlatformUsername: strings.TrimSpace(sourcePlatformUsername),
		}
	}

	for _, local := range manualUsers {
		if nodeID == "*" {
			addEntry("*", local, "local", "")
		} else {
			addEntry(nodeID, local, "local", "")
		}
	}

	selectedByBilling := make(map[string][]sshListPlatformAccount)
	for _, it := range req.PlatformAccounts {
		billing := strings.TrimSpace(it.BillingUsername)
		accNode := strings.TrimSpace(it.NodeID)
		accLocal := strings.TrimSpace(it.LocalUsername)
		if billing == "" || accNode == "" || accLocal == "" {
			continue
		}
		if nodeID != "*" && accNode != nodeID {
			continue
		}
		selectedByBilling[billing] = append(selectedByBilling[billing], sshListPlatformAccount{
			BillingUsername: billing,
			NodeID:          accNode,
			LocalUsername:   accLocal,
		})
		addEntry(accNode, accLocal, "platform", billing)
	}

	for _, billing := range billingUsers {
		if selected, ok := selectedByBilling[billing]; ok && len(selected) > 0 {
			continue
		}
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
			addEntry(accNode, accLocal, "platform", billing)
			matched++
		}
		if matched == 0 {
			if nodeID == "*" {
				addEntry("*", billing, "platform", billing)
			} else {
				addEntry(nodeID, billing, "platform", billing)
			}
		}
	}

	out := make([]sshListEntry, 0, len(entries))
	for _, v := range entries {
		out = append(out, v)
	}
	return out, nil
}

type sshUpsertResult struct {
	Applied               int      `json:"applied"`
	SkippedDueBlacklist   []string `json:"skipped_due_blacklist"`
	RemovedFromWhitelist  int      `json:"removed_from_whitelist"`
	RemovedFromExemptions int      `json:"removed_from_exemptions"`
}

func (s *Server) upsertWhitelistEntries(ctx context.Context, entries []sshListEntry, createdBy string, reason string) (sshUpsertResult, error) {
	result := sshUpsertResult{}
	allowed := make([]sshListEntry, 0, len(entries))
	for _, e := range entries {
		blacklisted, err := s.store.IsBlacklisted(ctx, e.NodeID, e.LocalUsername)
		if err != nil {
			return result, err
		}
		if blacklisted {
			result.SkippedDueBlacklist = append(result.SkippedDueBlacklist, e.NodeID+":"+e.LocalUsername)
			continue
		}
		allowed = append(allowed, e)
	}
	grouped := map[string][]string{}
	for _, e := range allowed {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertWhitelist(ctx, nodeID, trimUniq(users), createdBy, reason); err != nil {
			return result, err
		}
	}
	for _, e := range allowed {
		if err := s.store.UpsertSSHListSource(ctx, "whitelist", e.NodeID, e.LocalUsername, e.SourceType, e.SourcePlatformUsername, createdBy); err != nil {
			return result, err
		}
	}
	result.Applied = len(allowed)
	result.SkippedDueBlacklist = trimUniq(result.SkippedDueBlacklist)
	return result, nil
}

func (s *Server) upsertBlacklistEntries(ctx context.Context, entries []sshListEntry, createdBy string, reason string) (sshUpsertResult, error) {
	result := sshUpsertResult{}
	for _, e := range entries {
		if nodes, err := s.store.DeleteWhitelistWithNodes(ctx, e.NodeID, e.LocalUsername); err == nil {
			result.RemovedFromWhitelist += len(nodes)
		}
		_ = s.store.DeleteSSHListSource(ctx, "whitelist", e.NodeID, e.LocalUsername)
		if nodes, err := s.store.DeleteExemptionsWithNodes(ctx, e.NodeID, e.LocalUsername); err == nil {
			result.RemovedFromExemptions += len(nodes)
		}
		_ = s.store.DeleteSSHListSource(ctx, "exemptions", e.NodeID, e.LocalUsername)
	}
	grouped := map[string][]string{}
	for _, e := range entries {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertBlacklist(ctx, nodeID, trimUniq(users), createdBy, reason); err != nil {
			return result, err
		}
	}
	for _, e := range entries {
		if err := s.store.UpsertSSHListSource(ctx, "blacklist", e.NodeID, e.LocalUsername, e.SourceType, e.SourcePlatformUsername, createdBy); err != nil {
			return result, err
		}
	}
	result.Applied = len(entries)
	return result, nil
}

func (s *Server) upsertExemptionEntries(ctx context.Context, entries []sshListEntry, createdBy string, reason string) (sshUpsertResult, error) {
	result := sshUpsertResult{}
	allowed := make([]sshListEntry, 0, len(entries))
	for _, e := range entries {
		blacklisted, err := s.store.IsBlacklisted(ctx, e.NodeID, e.LocalUsername)
		if err != nil {
			return result, err
		}
		if blacklisted {
			result.SkippedDueBlacklist = append(result.SkippedDueBlacklist, e.NodeID+":"+e.LocalUsername)
			continue
		}
		allowed = append(allowed, e)
	}
	grouped := map[string][]string{}
	for _, e := range allowed {
		grouped[e.NodeID] = append(grouped[e.NodeID], e.LocalUsername)
	}
	for nodeID, users := range grouped {
		if err := s.store.UpsertExemptions(ctx, nodeID, trimUniq(users), createdBy, reason); err != nil {
			return result, err
		}
	}
	for _, e := range allowed {
		if err := s.store.UpsertSSHListSource(ctx, "exemptions", e.NodeID, e.LocalUsername, e.SourceType, e.SourcePlatformUsername, createdBy); err != nil {
			return result, err
		}
	}
	result.Applied = len(allowed)
	result.SkippedDueBlacklist = trimUniq(result.SkippedDueBlacklist)
	return result, nil
}

func (s *Server) fillSSHEntryDisplayMeta(ctx context.Context, nodeID string, listType string, whitelist []SSHWhitelistEntry, blacklist []SSHBlacklistEntry, exemptions []SSHExemptionEntry) error {
	sources, err := s.store.ListSSHListSources(ctx, listType, nodeID, 200000)
	if err != nil {
		return err
	}
	srcMap := make(map[string]SSHListSource, len(sources))
	for _, it := range sources {
		srcMap[it.NodeID+"|"+it.LocalUsername] = it
	}
	accounts, err := s.store.ListUserNodeAccounts(ctx, "", 200000)
	if err != nil {
		return err
	}
	accMap := make(map[string]string, len(accounts))
	localAnyMap := map[string]string{}
	for _, a := range accounts {
		key := strings.TrimSpace(a.NodeID) + "|" + strings.TrimSpace(a.LocalUsername)
		b := strings.TrimSpace(a.BillingUsername)
		if key != "|" && b != "" {
			accMap[key] = b
			localUser := strings.TrimSpace(a.LocalUsername)
			if _, ok := localAnyMap[localUser]; !ok {
				localAnyMap[localUser] = b
			}
		}
	}
	applyMeta := func(nodeID string, localUsername string, sourceType *string, sourcePlatformUsername *string, billingUsername *string) {
		k := strings.TrimSpace(nodeID) + "|" + strings.TrimSpace(localUsername)
		if src, ok := srcMap[k]; ok {
			*sourceType = strings.TrimSpace(src.SourceType)
			*sourcePlatformUsername = strings.TrimSpace(src.SourcePlatformUsername)
		}
		if strings.TrimSpace(*sourceType) != "platform" {
			*sourceType = "local"
			*sourcePlatformUsername = ""
		}
		if b, ok := accMap[k]; ok && b != "" {
			*billingUsername = b
		} else if strings.TrimSpace(nodeID) == "*" {
			if b, ok := localAnyMap[strings.TrimSpace(localUsername)]; ok && b != "" {
				*billingUsername = b
			}
		}
		if strings.TrimSpace(*billingUsername) == "" && strings.TrimSpace(*sourcePlatformUsername) != "" {
			*billingUsername = strings.TrimSpace(*sourcePlatformUsername)
		}
	}
	for i := range whitelist {
		applyMeta(whitelist[i].NodeID, whitelist[i].LocalUsername, &whitelist[i].SourceType, &whitelist[i].SourcePlatformUsername, &whitelist[i].BillingUsername)
	}
	for i := range blacklist {
		applyMeta(blacklist[i].NodeID, blacklist[i].LocalUsername, &blacklist[i].SourceType, &blacklist[i].SourcePlatformUsername, &blacklist[i].BillingUsername)
	}
	for i := range exemptions {
		applyMeta(exemptions[i].NodeID, exemptions[i].LocalUsername, &exemptions[i].SourceType, &exemptions[i].SourcePlatformUsername, &exemptions[i].BillingUsername)
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
	if err := s.fillSSHEntryDisplayMeta(c.Request.Context(), nodeID, "whitelist", rows, nil, nil); err != nil {
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
	result, err := s.upsertWhitelistEntries(c.Request.Context(), entries, s.currentOperator(c), strings.TrimSpace(req.Reason))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                    true,
		"entries":               len(entries),
		"applied":               result.Applied,
		"skipped_due_blacklist": result.SkippedDueBlacklist,
		"message":               "黑名单优先：与黑名单冲突的账号已自动跳过",
	})
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
		_ = s.store.DeleteSSHListSource(c.Request.Context(), "whitelist", n, localUsername)
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
	if err := s.fillSSHEntryDisplayMeta(c.Request.Context(), nodeID, "blacklist", nil, rows, nil); err != nil {
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
	result, err := s.upsertBlacklistEntries(c.Request.Context(), entries, operator, strings.TrimSpace(req.Reason))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, e := range entries {
		s.enqueueKickSSHUser(c.Request.Context(), e.NodeID, e.LocalUsername, fmt.Sprintf("管理员 %s 加入 SSH 黑名单，已强制断开该账号 SSH 会话", operator))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                      true,
		"entries":                 len(entries),
		"applied":                 result.Applied,
		"kicked":                  true,
		"removed_from_whitelist":  result.RemovedFromWhitelist,
		"removed_from_exemptions": result.RemovedFromExemptions,
		"message":                 "黑名单优先：冲突项已从白名单/豁免名单自动移除",
	})
}

func (s *Server) handleAdminBlacklistDelete(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	nodes, err := s.store.DeleteBlacklistWithNodes(c.Request.Context(), nodeID, localUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, n := range nodes {
		_ = s.store.DeleteSSHListSource(c.Request.Context(), "blacklist", n, localUsername)
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
	if err := s.fillSSHEntryDisplayMeta(c.Request.Context(), nodeID, "exemptions", nil, nil, rows); err != nil {
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
	result, err := s.upsertExemptionEntries(c.Request.Context(), entries, operator, strings.TrimSpace(req.Reason))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                    true,
		"entries":               len(entries),
		"applied":               result.Applied,
		"skipped_due_blacklist": result.SkippedDueBlacklist,
		"message":               "黑名单优先：与黑名单冲突的账号已自动跳过",
	})
}

func (s *Server) handleAdminExemptionsDelete(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	nodes, err := s.store.DeleteExemptionsWithNodes(c.Request.Context(), nodeID, localUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, n := range nodes {
		_ = s.store.DeleteSSHListSource(c.Request.Context(), "exemptions", n, localUsername)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type powerUserCreateReq struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	CanViewBoard      bool   `json:"can_view_board"`
	CanViewNodes      bool   `json:"can_view_nodes"`
	CanManageNodes    bool   `json:"can_manage_nodes"`
	CanReviewRequests bool   `json:"can_review_requests"`
}

type powerUserPromoteReq struct {
	Username          string `json:"username"`
	CanViewBoard      bool   `json:"can_view_board"`
	CanViewNodes      bool   `json:"can_view_nodes"`
	CanManageNodes    bool   `json:"can_manage_nodes"`
	CanReviewRequests bool   `json:"can_review_requests"`
}

type powerUserPermReq struct {
	CanViewBoard      bool `json:"can_view_board"`
	CanViewNodes      bool `json:"can_view_nodes"`
	CanManageNodes    bool `json:"can_manage_nodes"`
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
	if err := s.store.CreatePowerUser(c.Request.Context(), req.Username, req.Password, req.CanViewBoard, req.CanViewNodes, req.CanManageNodes, req.CanReviewRequests, createdBy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminPowerUsersPromote(c *gin.Context) {
	var req powerUserPromoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	operator := strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_user")))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.PromotePlatformUserToPowerUser(
		c.Request.Context(),
		req.Username,
		req.CanViewBoard,
		req.CanViewNodes,
		req.CanManageNodes,
		req.CanReviewRequests,
		operator,
	); err != nil {
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
	if err := s.store.UpdatePowerUserPermissions(c.Request.Context(), username, req.CanViewBoard, req.CanViewNodes, req.CanManageNodes, req.CanReviewRequests, updatedBy); err != nil {
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

func (s *Server) handleAdminPowerUsersDemote(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username 不能为空"})
		return
	}
	if err := s.store.DemotePowerUserToPlatformUser(c.Request.Context(), username); err != nil {
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
	visibleNodes, restricted, err := s.visibleNodeIDsForPowerUser(c)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if restricted && len(visibleNodes) == 0 {
		c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": []UsageUserSummary{}})
		return
	}
	rows, err := s.store.ListUsageSummaryByUser(c.Request.Context(), from, to, limit, visibleNodes)
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
	visibleNodes, restricted, err := s.visibleNodeIDsForPowerUser(c)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if restricted && len(visibleNodes) == 0 {
		c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": []PlatformUsageUserSummary{}})
		return
	}
	rows, err := s.store.ListPlatformUsageSummaryByUser(c.Request.Context(), from, to, limit, visibleNodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if getAuthRole(c) == "power_user" {
		adminSet, setErr := s.adminUsernameSet(c.Request.Context())
		if setErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": setErr.Error()})
			return
		}
		filtered := make([]PlatformUsageUserSummary, 0, len(rows))
		for _, r := range rows {
			if _, isAdmin := adminSet[strings.TrimSpace(r.PlatformUsername)]; isAdmin {
				continue
			}
			filtered = append(filtered, r)
		}
		rows = filtered
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
	if getAuthRole(c) == "power_user" {
		isAdmin, aErr := s.store.IsAdminAccount(c.Request.Context(), username)
		if aErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": aErr.Error()})
			return
		}
		if isAdmin {
			c.JSON(http.StatusNotFound, gin.H{"error": "平台账号不存在"})
			return
		}
	}
	limit := parseLimit(c.Query("limit"), 2000, 20000)
	visibleNodes, restricted, err := s.visibleNodeIDsForPowerUser(c)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if restricted && len(visibleNodes) == 0 {
		c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "username": username, "rows": []PlatformUsageNodeDetail{}})
		return
	}
	rows, err := s.store.ListPlatformUsageNodeDetails(c.Request.Context(), username, from, to, limit, visibleNodes)
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
	visibleNodes, restricted, err := s.visibleNodeIDsForPowerUser(c)
	if err != nil {
		if strings.TrimSpace(err.Error()) == "forbidden" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if restricted && len(visibleNodes) == 0 {
		c.JSON(http.StatusOK, gin.H{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": []UsageMonthlySummary{}})
		return
	}
	rows, err := s.store.ListUsageMonthlyByUser(c.Request.Context(), from, to, limit, visibleNodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if getAuthRole(c) == "power_user" {
		adminSet, setErr := s.adminUsernameSet(c.Request.Context())
		if setErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": setErr.Error()})
			return
		}
		filtered := make([]UsageMonthlySummary, 0, len(rows))
		for _, r := range rows {
			if _, isAdmin := adminSet[strings.TrimSpace(r.Username)]; isAdmin {
				continue
			}
			filtered = append(filtered, r)
		}
		rows = filtered
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
	if getAuthRole(c) == "power_user" {
		adminSet, setErr := s.adminUsernameSet(c.Request.Context())
		if setErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": setErr.Error()})
			return
		}
		filtered := make([]RechargeSummary, 0, len(rows))
		for _, r := range rows {
			if _, isAdmin := adminSet[strings.TrimSpace(r.Username)]; isAdmin {
				continue
			}
			filtered = append(filtered, r)
		}
		rows = filtered
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
	if getAuthRole(c) == "power_user" {
		username := strings.TrimSpace(c.GetString("auth_user"))
		visible, err := s.store.ListVisibleNodeIDsForPowerUser(c.Request.Context(), username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibleSet := make(map[string]struct{}, len(visible))
		for _, id := range visible {
			visibleSet[id] = struct{}{}
		}
		filtered := make([]NodeStatus, 0, len(nodes))
		for _, n := range nodes {
			if _, ok := visibleSet[strings.TrimSpace(n.NodeID)]; ok {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

func (s *Server) handleAdminNodeDetail(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if !s.ensureNodeReadable(c, nodeID) {
		return
	}
	minutes := 180
	if v := strings.TrimSpace(c.Query("minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			minutes = n
		}
	}
	if minutes <= 0 {
		minutes = 180
	}
	if minutes > 24*7*60 {
		minutes = 24 * 7 * 60
	}
	limit := parseLimit(c.Query("limit"), 360, 5000)

	node, err := s.store.GetNodeStatus(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 统一使用 UTC，避免本地时区导致 report_ts 时间窗口错位，出现 history 为空。
	to := time.Now().UTC()
	from := to.Add(-time.Duration(minutes) * time.Minute)
	history, err := s.store.ListNodeRuntimeSnapshots(c.Request.Context(), nodeID, from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	latest := NodeRuntimeSnapshot{}
	if len(history) > 0 {
		latest = history[0]
	} else {
		// 兼容：节点状态已更新但快照尚未入库时，至少返回 SSH 在线人数，避免前端显示 0。
		latest.NodeID = node.NodeID
		latest.ReportTS = node.LastReportTS
		latest.SSHUserCount = node.SSHActiveCount
		latest.GPUProcessCount = node.GPUProcessCount
		latest.CPUProcessCount = node.CPUProcessCount
		latest.CostTotal = node.CostTotal
	}
	// 返回升序，便于前端直接画时间序列图
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	localUsers, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 3000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nowLocal := time.Now()
	monthStart := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, nowLocal.Location())
	monthlyUserCosts, err := s.store.ListNodeMonthlyUserCosts(c.Request.Context(), nodeID, monthStart, nowLocal, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	securityEvents, err := s.store.ListNodeSecurityEvents(c.Request.Context(), nodeID, "", nil, nil, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	suspiciousUsers, err := s.store.ListNodeSuspiciousUsers(c.Request.Context(), nodeID, 7, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"node":               node,
		"latest":             latest,
		"history":            history,
		"local_users":        localUsers,
		"monthly_user_costs": monthlyUserCosts,
		"monthly_from":       monthStart.Format(time.RFC3339),
		"monthly_to":         nowLocal.Format(time.RFC3339),
		"security_events":    securityEvents,
		"suspicious_users":   suspiciousUsers,
		"from":               from.Format(time.RFC3339),
		"to":                 to.Format(time.RFC3339),
	})
}

func (s *Server) resolveNodePointsInterceptPolicyValues(p NodePointsInterceptPolicy) (float64, float64, float64, float64) {
	threshold := s.cfg.LimitedThreshold
	limited := s.cfg.CPULimitPercentLimited
	blocked := s.cfg.CPULimitPercentBlocked
	overdraftMemory := s.cfg.OverdraftMemoryLimitGB
	if p.ThrottleThresholdPoints != nil {
		threshold = *p.ThrottleThresholdPoints
	}
	if p.LimitedCPUQuotaPercent != nil {
		limited = *p.LimitedCPUQuotaPercent
	}
	if p.BlockedCPUQuotaPercent != nil {
		blocked = *p.BlockedCPUQuotaPercent
	}
	if p.OverdraftMemoryLimitGB != nil {
		overdraftMemory = *p.OverdraftMemoryLimitGB
	}
	if overdraftMemory < 0 {
		overdraftMemory = 0
	}
	return threshold, limited, blocked, overdraftMemory
}

func normalizeNodeQuotaMounts(mounts []string) []string {
	cleaned := uniqTrim(mounts)
	out := make([]string, 0, len(cleaned))
	for _, m := range cleaned {
		x := strings.TrimSpace(m)
		if x == "" {
			continue
		}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		score := func(v string) int {
			switch strings.TrimSpace(v) {
			case "/home":
				return 0
			case "/":
				return 1
			default:
				return 2
			}
		}
		si := score(out[i])
		sj := score(out[j])
		if si != sj {
			return si < sj
		}
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func pickPreferredQuotaMount(mounts []string) string {
	out := normalizeNodeQuotaMounts(mounts)
	if len(out) == 0 {
		return ""
	}
	return out[0]
}

func containsQuotaMount(mounts []string, mount string) bool {
	mount = strings.TrimSpace(mount)
	if mount == "" {
		return false
	}
	for _, x := range mounts {
		if strings.TrimSpace(x) == mount {
			return true
		}
	}
	return false
}

func resolveDiskQuotaPolicyValues(p NodeDiskQuotaPolicy) (string, float64, float64) {
	mountpoint := strings.TrimSpace(p.Mountpoint)
	soft := 0.0
	hard := 0.0
	if p.DefaultSoft != nil {
		soft = *p.DefaultSoft
	}
	if p.DefaultHard != nil {
		hard = *p.DefaultHard
	}
	return mountpoint, soft, hard
}

func (s *Server) parseSecurityEventRange(c *gin.Context) (*time.Time, *time.Time, error) {
	var fromPtr *time.Time
	var toPtr *time.Time
	if x := strings.TrimSpace(c.Query("from")); x != "" {
		t, err := parseTimeFlexible(x)
		if err != nil {
			return nil, nil, fmt.Errorf("from 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD")
		}
		fromPtr = &t
	}
	if x := strings.TrimSpace(c.Query("to")); x != "" {
		t, err := parseTimeFlexible(x)
		if err != nil {
			return nil, nil, fmt.Errorf("to 时间格式不合法，建议 RFC3339 或 YYYY-MM-DD")
		}
		if len(x) == len("2006-01-02") {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		toPtr = &t
	}
	if fromPtr != nil && toPtr != nil && toPtr.Before(*fromPtr) {
		return nil, nil, fmt.Errorf("to 不能早于 from")
	}
	return fromPtr, toPtr, nil
}

func (s *Server) handleAdminNodeSecurityEvents(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if !s.ensureNodeReadable(c, nodeID) {
		return
	}
	eventType := strings.TrimSpace(c.Query("event_type"))
	limit := parseLimit(c.Query("limit"), 200, 2000)
	summaryLimit := parseLimit(c.Query("summary_limit"), 120, 1000)
	from, to, err := s.parseSecurityEventRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	events, err := s.store.ListNodeSecurityEvents(c.Request.Context(), nodeID, eventType, from, to, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	summaries, err := s.store.ListNodeSecurityEventSummaries(c.Request.Context(), nodeID, eventType, from, to, summaryLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	suspiciousDays := 7
	rangeFrom := time.Now().Add(-7 * 24 * time.Hour)
	rangeTo := time.Now()
	if from != nil || to != nil {
		if from != nil {
			rangeFrom = *from
		}
		if to != nil {
			rangeTo = *to
		}
		if rangeTo.Before(rangeFrom) {
			rangeTo = rangeFrom
		}
		days := int(rangeTo.Sub(rangeFrom).Hours()/24.0) + 1
		if days < 1 {
			days = 1
		}
		if days > 365 {
			days = 365
		}
		suspiciousDays = days
	}
	var users []NodeSuspiciousUser
	if from != nil || to != nil {
		users, err = s.store.ListNodeSuspiciousUsersByRange(c.Request.Context(), nodeID, rangeFrom, rangeTo, 500)
	} else {
		users, err = s.store.ListNodeSuspiciousUsers(c.Request.Context(), nodeID, suspiciousDays, 500)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var fromText string
	var toText string
	if from != nil {
		fromText = from.UTC().Format(time.RFC3339)
	}
	if to != nil {
		toText = to.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":            nodeID,
		"event_type":         eventType,
		"events":             events,
		"event_summaries":    summaries,
		"suspicious_users":   users,
		"suspicious_days":    suspiciousDays,
		"from":               fromText,
		"to":                 toText,
		"summary_normalizer": "event_type + severity + reason(数字归一化)",
	})
}

type adminNodeSSHGuardSetReq struct {
	Enabled bool `json:"enabled"`
}

type adminNodePointsInterceptSetReq struct {
	Enabled                 bool     `json:"enabled"`
	ThrottleThresholdPoints *float64 `json:"throttle_threshold_points"`
	LimitedCPUQuotaPercent  *float64 `json:"limited_cpu_quota_percent"`
	BlockedCPUQuotaPercent  *float64 `json:"blocked_cpu_quota_percent"`
	OverdraftMemoryLimitGB  *float64 `json:"overdraft_memory_limit_gb"`
}

type adminNodeDiskQuotaSetReq struct {
	Enabled       bool     `json:"enabled"`
	Mountpoint    string   `json:"mountpoint"`
	DefaultSoftMB *float64 `json:"default_soft_mb"`
	DefaultHardMB *float64 `json:"default_hard_mb"`
	ApplyToAll    bool     `json:"apply_to_all"`
}

type adminNodeDiskQuotaApplyUserReq struct {
	LocalUsername string  `json:"local_username"`
	SoftMB        float64 `json:"soft_mb"`
	HardMB        float64 `json:"hard_mb"`
}

type adminNodeDiskQuotaApplyReq struct {
	Mountpoint string                           `json:"mountpoint"`
	AllUsers   bool                             `json:"all_users"`
	SoftMB     *float64                         `json:"soft_mb"`
	HardMB     *float64                         `json:"hard_mb"`
	Users      []adminNodeDiskQuotaApplyUserReq `json:"users"`
}

type adminNodeModelPriceOverride struct {
	GPUModel       string  `json:"gpu_model"`
	PricePerMinute float64 `json:"price_per_minute"`
}

type adminNodePriceSetReq struct {
	PricePerMinute      *float64                      `json:"price_per_minute"`
	UseDefault          bool                          `json:"use_default"`
	ModelPriceOverrides []adminNodeModelPriceOverride `json:"model_price_overrides"`
}

func nodeModelPriceOverridesToRespRows(in map[string]float64) []gin.H {
	if len(in) == 0 {
		return []gin.H{}
	}
	keys := make([]string, 0, len(in))
	for model := range in {
		keys = append(keys, model)
	}
	sort.Strings(keys)
	out := make([]gin.H, 0, len(keys))
	for _, model := range keys {
		out = append(out, gin.H{
			"gpu_model":        model,
			"price_per_minute": in[model],
		})
	}
	return out
}

func (s *Server) handleAdminNodeSSHGuardSet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	var req adminNodeSSHGuardSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prevEnabled, err := s.store.IsNodeSSHGuardEnabled(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.UpsertNodeSSHGuardPolicy(c.Request.Context(), nodeID, req.Enabled, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	kickTriggered := false
	if !prevEnabled && req.Enabled {
		kickTriggered = true
		s.enqueueNodeAction(nodeID, Action{
			Type:   "kick_ssh_all",
			Reason: fmt.Sprintf("管理员 %s 开启了未注册用户拦截，要求全部 SSH 会话重新登录", operator),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"node_id":        nodeID,
		"enabled":        req.Enabled,
		"previous":       prevEnabled,
		"kick_triggered": kickTriggered,
	})
}

func (s *Server) handleAdminNodePointsInterceptGet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	policy, err := s.store.GetNodePointsInterceptPolicy(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	threshold, limitedQuota, blockedQuota, overdraftMemoryLimit := s.resolveNodePointsInterceptPolicyValues(policy)
	c.JSON(http.StatusOK, gin.H{
		"ok":                            true,
		"node_id":                       nodeID,
		"enabled":                       policy.Enabled,
		"throttle_threshold_points":     policy.ThrottleThresholdPoints,
		"limited_cpu_quota_percent":     policy.LimitedCPUQuotaPercent,
		"blocked_cpu_quota_percent":     policy.BlockedCPUQuotaPercent,
		"overdraft_memory_limit_gb":     policy.OverdraftMemoryLimitGB,
		"effective_threshold_points":    threshold,
		"effective_limited_cpu_quota":   limitedQuota,
		"effective_blocked_cpu_quota":   blockedQuota,
		"effective_overdraft_memory_gb": overdraftMemoryLimit,
		"default_threshold_points":      s.cfg.LimitedThreshold,
		"default_limited_cpu_quota":     s.cfg.CPULimitPercentLimited,
		"default_blocked_cpu_quota":     s.cfg.CPULimitPercentBlocked,
		"default_overdraft_memory_gb":   s.cfg.OverdraftMemoryLimitGB,
		"cpu_control_enabled_on_server": s.cfg.EnableCPUControl,
	})
}

func (s *Server) handleAdminNodePointsInterceptSet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req adminNodePointsInterceptSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prevPolicy, err := s.store.GetNodePointsInterceptPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.UpsertNodePointsInterceptPolicy(
		c.Request.Context(),
		nodeID,
		req.Enabled,
		req.ThrottleThresholdPoints,
		req.LimitedCPUQuotaPercent,
		req.BlockedCPUQuotaPercent,
		req.OverdraftMemoryLimitGB,
		operator,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updatedPolicy, err := s.store.GetNodePointsInterceptPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	threshold, limitedQuota, blockedQuota, overdraftMemoryLimit := s.resolveNodePointsInterceptPolicyValues(updatedPolicy)
	quotaResetTargets := 0
	quotaSyncTargets := 0
	memorySyncTargets := 0
	monthlyCfg, err := s.store.GetMonthlyPointsConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 策略变更后立即重算并下发 CPU 配额，避免必须等待下一次采集上报才生效。
	if localUsers, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 5000); err == nil {
		seen := map[string]struct{}{}
		for _, u := range localUsers {
			local := strings.TrimSpace(u.LocalUsername)
			if local == "" {
				continue
			}
			if _, ok := seen[local]; ok {
				continue
			}
			seen[local] = struct{}{}

			targetQuota := 0.0
			targetMemoryGB := 0.0
			reason := fmt.Sprintf("管理员 %s 更新节点积分限速策略，解除 CPU 限制", operator)
			memoryReason := fmt.Sprintf("管理员 %s 更新节点积分限速策略，解除内存限额", operator)
			if req.Enabled {
				billing := strings.TrimSpace(u.PlatformUsername)
				if billing == "" {
					continue
				}
				pu, err := s.store.GetUser(c.Request.Context(), billing)
				if err != nil {
					continue
				}
				effectiveBalance := pu.Balance + pu.CarryoverBalance
				switch {
				case effectiveBalance <= 0:
					targetQuota = blockedQuota
					reason = fmt.Sprintf("管理员 %s 更新节点积分限速策略：余额 %.2f<=0，执行欠费强限速（%.2f%%）", operator, effectiveBalance, blockedQuota)
				case effectiveBalance <= threshold:
					targetQuota = limitedQuota
					reason = fmt.Sprintf("管理员 %s 更新节点积分限速策略：余额 %.2f<=阈值 %.2f，执行低积分限速（%.2f%%）", operator, effectiveBalance, threshold, limitedQuota)
				default:
					targetQuota = 0
				}
				if isOverdraftExceeded(effectiveBalance, monthlyCfg.MaxOverdraftLimit) && overdraftMemoryLimit > 0 {
					targetMemoryGB = overdraftMemoryLimit
					memoryReason = fmt.Sprintf("管理员 %s 更新节点积分限速策略：余额 %.2f 已超过每月欠费上限，内存上限设为 %.2f GB", operator, effectiveBalance, targetMemoryGB)
				} else {
					memoryReason = fmt.Sprintf("管理员 %s 更新节点积分限速策略：余额 %.2f 未超过每月欠费上限，解除内存限额", operator, effectiveBalance)
				}
			} else {
				reason = fmt.Sprintf("管理员 %s 关闭积分拦截，按节点策略解除 CPU 限速", operator)
				memoryReason = fmt.Sprintf("管理员 %s 关闭积分拦截，按节点策略解除内存限额", operator)
			}

			if s.enqueueCPUQuotaAction(nodeID, local, targetQuota, reason, true) {
				quotaSyncTargets++
				if !req.Enabled {
					quotaResetTargets++
				}
			}
			if action, ok := s.nextMemoryLimitAction(nodeID, local, targetMemoryGB, memoryReason, true); ok {
				s.enqueueNodeAction(nodeID, action)
				memorySyncTargets++
			}
		}
	}
	// 语义：关闭积分拦截开关 => 不扣分且不限速，需要主动清理本节点历史 CPU 限速残留。
	if !req.Enabled {
		s.clearNodeCPUQuotaState(nodeID)
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 更新积分拦截策略，立即刷新节点策略", operator),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":                            true,
		"node_id":                       nodeID,
		"enabled":                       updatedPolicy.Enabled,
		"previous":                      prevPolicy.Enabled,
		"quota_reset_targets":           quotaResetTargets,
		"quota_sync_targets":            quotaSyncTargets,
		"throttle_threshold_points":     updatedPolicy.ThrottleThresholdPoints,
		"limited_cpu_quota_percent":     updatedPolicy.LimitedCPUQuotaPercent,
		"blocked_cpu_quota_percent":     updatedPolicy.BlockedCPUQuotaPercent,
		"overdraft_memory_limit_gb":     updatedPolicy.OverdraftMemoryLimitGB,
		"effective_threshold_points":    threshold,
		"effective_limited_cpu_quota":   limitedQuota,
		"effective_blocked_cpu_quota":   blockedQuota,
		"effective_overdraft_memory_gb": overdraftMemoryLimit,
		"memory_sync_targets":           memorySyncTargets,
	})
}

func (s *Server) handleAdminNodeDiskQuotaGet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if !s.ensureNodeReadable(c, nodeID) {
		return
	}
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
	policy, err := s.store.GetNodeDiskQuotaPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	mounts := normalizeNodeQuotaMounts(node.DiskQuotaMounts)
	preferredMount := pickPreferredQuotaMount(mounts)
	policyMount, defaultSoft, defaultHard := resolveDiskQuotaPolicyValues(policy)
	effectiveMount := policyMount
	if effectiveMount == "" {
		effectiveMount = preferredMount
	}
	localUsers, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	userQuotas, err := s.store.ListNodeUserDiskQuotas(c.Request.Context(), nodeID, effectiveMount, 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	quotaMap := make(map[string]NodeUserDiskQuota, len(userQuotas))
	for _, q := range userQuotas {
		local := strings.TrimSpace(q.LocalUsername)
		if local == "" {
			continue
		}
		quotaMap[local] = q
	}
	for i := range localUsers {
		local := strings.TrimSpace(localUsers[i].LocalUsername)
		if local == "" {
			continue
		}
		if q, ok := quotaMap[local]; ok {
			localUsers[i].QuotaMountpoint = q.Mountpoint
			vUsed := q.UsedMB
			vSoft := q.SoftMB
			vHard := q.HardMB
			localUsers[i].QuotaUsedMB = &vUsed
			localUsers[i].QuotaSoftMB = &vSoft
			localUsers[i].QuotaHardMB = &vHard
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                   true,
		"node_id":              nodeID,
		"quota_installed":      node.DiskQuotaInstalled,
		"quota_mounts":         mounts,
		"preferred_mountpoint": preferredMount,
		"effective_mountpoint": effectiveMount,
		"enabled":              policy.Enabled,
		"mountpoint":           policy.Mountpoint,
		"default_soft_mb":      policy.DefaultSoft,
		"default_hard_mb":      policy.DefaultHard,
		"effective_soft_mb":    defaultSoft,
		"effective_hard_mb":    defaultHard,
		"users":                localUsers,
		"checked_at":           node.LastReportTS.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminNodeDiskQuotaSet(c *gin.Context) {
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
	var req adminNodeDiskQuotaSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mounts := normalizeNodeQuotaMounts(node.DiskQuotaMounts)
	if x := strings.TrimSpace(req.Mountpoint); x != "" && len(mounts) > 0 && !containsQuotaMount(mounts, x) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        fmt.Sprintf("节点未检测到分区 %s 的 quota，已检测分区：%s", x, strings.Join(mounts, ", ")),
			"quota_mounts": mounts,
		})
		return
	}
	prevPolicy, err := s.store.GetNodeDiskQuotaPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 预估“更新后策略”，用于 apply_to_all 的参数校验。
	nextMount := strings.TrimSpace(req.Mountpoint)
	if nextMount == "" {
		nextMount = strings.TrimSpace(prevPolicy.Mountpoint)
	}
	if nextMount == "" {
		nextMount = pickPreferredQuotaMount(mounts)
	}
	var nextSoft *float64
	var nextHard *float64
	if req.DefaultSoftMB != nil {
		v := *req.DefaultSoftMB
		nextSoft = &v
	} else if prevPolicy.DefaultSoft != nil {
		v := *prevPolicy.DefaultSoft
		nextSoft = &v
	}
	if req.DefaultHardMB != nil {
		v := *req.DefaultHardMB
		nextHard = &v
	} else if prevPolicy.DefaultHard != nil {
		v := *prevPolicy.DefaultHard
		nextHard = &v
	}
	_, _, _, normErr := normalizeDiskQuotaPolicyInput(nextMount, nextSoft, nextHard)
	if normErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": normErr.Error()})
		return
	}
	if req.ApplyToAll && req.Enabled {
		if nextMount == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未检测到可用 quota 分区，请先在节点开启 quota 并等待上报"})
			return
		}
	}

	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.UpsertNodeDiskQuotaPolicy(
		c.Request.Context(),
		nodeID,
		req.Enabled,
		req.Mountpoint,
		req.DefaultSoftMB,
		req.DefaultHardMB,
		operator,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updatedPolicy, err := s.store.GetNodeDiskQuotaPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	effectiveMount, effectiveSoft, effectiveHard := resolveDiskQuotaPolicyValues(updatedPolicy)
	if effectiveMount == "" {
		effectiveMount = pickPreferredQuotaMount(mounts)
	}
	appliedUsers := 0
	skippedAdminMappings := 0
	if req.ApplyToAll && updatedPolicy.Enabled && effectiveMount != "" {
		if localUsers, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 5000); err == nil {
			seen := map[string]struct{}{}
			adminCache := map[string]bool{}
			for _, u := range localUsers {
				local := strings.TrimSpace(u.LocalUsername)
				if local == "" {
					continue
				}
				if _, ok := seen[local]; ok {
					continue
				}
				seen[local] = struct{}{}
				adminMapped, checkErr := s.shouldSkipAdminMappedDiskQuotaTarget(c.Request.Context(), nodeID, u, adminCache)
				if checkErr == nil && adminMapped {
					skippedAdminMappings++
					continue
				}
				if checkErr != nil {
					log.Printf("skip disk quota target check failed node=%s local=%s err=%v", nodeID, local, checkErr)
				}
				s.enqueueNodeAction(nodeID, Action{
					Type:                "set_disk_quota",
					Username:            local,
					DiskQuotaMountpoint: effectiveMount,
					DiskQuotaSoftMB:     effectiveSoft,
					DiskQuotaHardMB:     effectiveHard,
					Reason:              fmt.Sprintf("管理员 %s 更新节点磁盘配额策略（全体应用）", operator),
				})
				appliedUsers++
			}
		}
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 更新节点磁盘配额策略，立即同步节点状态", operator),
	})
	warning := ""
	if !node.DiskQuotaInstalled {
		warning = "节点尚未检测到 quota 工具，策略可保存但下发动作可能失败"
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                   true,
		"node_id":              nodeID,
		"enabled":              updatedPolicy.Enabled,
		"mountpoint":           updatedPolicy.Mountpoint,
		"default_soft_mb":      updatedPolicy.DefaultSoft,
		"default_hard_mb":      updatedPolicy.DefaultHard,
		"effective_mountpoint": effectiveMount,
		"effective_soft_mb":    effectiveSoft,
		"effective_hard_mb":    effectiveHard,
		"quota_installed":      node.DiskQuotaInstalled,
		"quota_mounts":         mounts,
		"applied_users":        appliedUsers,
		"skipped_admin_users":  skippedAdminMappings,
		"warning":              warning,
	})
}

func (s *Server) handleAdminNodeDiskQuotaApply(c *gin.Context) {
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
	var req adminNodeDiskQuotaApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy, err := s.store.GetNodeDiskQuotaPolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	mounts := normalizeNodeQuotaMounts(node.DiskQuotaMounts)
	mountpoint := strings.TrimSpace(req.Mountpoint)
	if mountpoint == "" {
		mountpoint = strings.TrimSpace(policy.Mountpoint)
	}
	if mountpoint == "" {
		mountpoint = pickPreferredQuotaMount(mounts)
	}
	if mountpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未检测到可用 quota 分区，请先在节点开启 quota 并等待上报"})
		return
	}
	if len(mounts) > 0 && !containsQuotaMount(mounts, mountpoint) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        fmt.Sprintf("节点未检测到分区 %s 的 quota，已检测分区：%s", mountpoint, strings.Join(mounts, ", ")),
			"quota_mounts": mounts,
		})
		return
	}

	type target struct {
		local string
		soft  float64
		hard  float64
	}
	targets := make([]target, 0)
	if req.AllUsers {
		soft := 0.0
		hard := 0.0
		if req.SoftMB != nil {
			soft = *req.SoftMB
		} else if policy.DefaultSoft != nil {
			soft = *policy.DefaultSoft
		}
		if req.HardMB != nil {
			hard = *req.HardMB
		} else if policy.DefaultHard != nil {
			hard = *policy.DefaultHard
		}
		_, normSoft, normHard, normErr := normalizeDiskQuotaPolicyInput(mountpoint, &soft, &hard)
		if normErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": normErr.Error()})
			return
		}
		if normSoft == nil || normHard == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "全体应用时软/硬配额不能为空"})
			return
		}
		localUsers, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 5000)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		seen := map[string]struct{}{}
		for _, u := range localUsers {
			local := strings.TrimSpace(u.LocalUsername)
			if local == "" {
				continue
			}
			if _, ok := seen[local]; ok {
				continue
			}
			seen[local] = struct{}{}
			targets = append(targets, target{local: local, soft: *normSoft, hard: *normHard})
		}
	} else {
		if len(req.Users) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "users 不能为空"})
			return
		}
		for _, u := range req.Users {
			local := strings.TrimSpace(u.LocalUsername)
			_, normSoft, normHard, normErr := normalizeDiskQuotaPolicyInput(mountpoint, &u.SoftMB, &u.HardMB)
			if local == "" || normErr != nil || normSoft == nil || normHard == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "users 中存在非法参数（local_username/soft_mb/hard_mb）"})
				return
			}
			targets = append(targets, target{local: local, soft: *normSoft, hard: *normHard})
		}
	}

	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	applied := 0
	skippedAdminMappings := 0
	seen := map[string]struct{}{}
	adminCache := map[string]bool{}
	localMeta := map[string]NodeLocalUser{}
	if req.AllUsers {
		if rows, err := s.store.ListNodeLocalUsersWithPlatformMapping(c.Request.Context(), nodeID, 5000); err == nil {
			for _, it := range rows {
				local := strings.TrimSpace(it.LocalUsername)
				if local == "" {
					continue
				}
				localMeta[local] = it
			}
		}
	}
	for _, t := range targets {
		if _, ok := seen[t.local]; ok {
			continue
		}
		seen[t.local] = struct{}{}
		if req.AllUsers {
			meta, ok := localMeta[t.local]
			if ok {
				adminMapped, checkErr := s.shouldSkipAdminMappedDiskQuotaTarget(c.Request.Context(), nodeID, meta, adminCache)
				if checkErr == nil && adminMapped {
					skippedAdminMappings++
					continue
				}
				if checkErr != nil {
					log.Printf("skip disk quota target check failed node=%s local=%s err=%v", nodeID, t.local, checkErr)
				}
			}
		}
		s.enqueueNodeAction(nodeID, Action{
			Type:                "set_disk_quota",
			Username:            t.local,
			DiskQuotaMountpoint: mountpoint,
			DiskQuotaSoftMB:     t.soft,
			DiskQuotaHardMB:     t.hard,
			Reason:              fmt.Sprintf("管理员 %s 手动应用节点磁盘配额", operator),
		})
		applied++
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 下发节点磁盘配额，立即同步节点状态", operator),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":                  true,
		"node_id":             nodeID,
		"mountpoint":          mountpoint,
		"applied_users":       applied,
		"skipped_admin_users": skippedAdminMappings,
		"quota_installed":     node.DiskQuotaInstalled,
		"quota_mounts":        mounts,
		"message":             "已下发磁盘配额动作，节点会在约 1 秒内执行",
	})
}

func (s *Server) handleAdminNodePriceGet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	price, modelOverrides, err := s.store.GetNodePricePolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	mode := "default"
	if price != nil {
		mode = "custom"
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                       true,
		"node_id":                  nodeID,
		"price_per_minute":         price,
		"model_price_overrides":    nodeModelPriceOverridesToRespRows(modelOverrides),
		"mode":                     mode,
		"default_price_per_minute": s.cfg.DefaultPricePerMinute,
	})
}

func (s *Server) handleAdminNodePriceSet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req adminNodePriceSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.PricePerMinute != nil && *req.PricePerMinute < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price_per_minute 不能为负数"})
		return
	}
	if req.PricePerMinute == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price_per_minute 不能为空（请按节点设置单价）"})
		return
	}
	if req.UseDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "已不支持 use_default，请直接设置该节点单价"})
		return
	}
	if len(req.ModelPriceOverrides) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "已不支持 model_price_overrides，请仅设置节点单价"})
		return
	}

	prevPrice, prevModelOverrides, err := s.store.GetNodePricePolicy(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetPrice := req.PricePerMinute
	targetModelOverrides := map[string]float64{}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.UpsertNodePricePolicy(c.Request.Context(), nodeID, targetPrice, targetModelOverrides, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	mode := "custom"
	c.JSON(http.StatusOK, gin.H{
		"ok":                             true,
		"node_id":                        nodeID,
		"previous_price_per_minute":      prevPrice,
		"previous_model_price_overrides": nodeModelPriceOverridesToRespRows(prevModelOverrides),
		"price_per_minute":               targetPrice,
		"model_price_overrides":          nodeModelPriceOverridesToRespRows(targetModelOverrides),
		"mode":                           mode,
		"default_price_per_minute":       s.cfg.DefaultPricePerMinute,
	})
}

func (s *Server) handleAdminNodeSSHExclusiveGet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	enabled, err := s.store.GetNodeSSHExclusivePolicy(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	users, err := s.store.ListNodeExclusiveUsers(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidates, err := s.store.ListNodeExclusiveCandidates(c.Request.Context(), nodeID, 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":               nodeID,
		"enabled":               enabled,
		"exclusive_users":       users,
		"candidate_local_users": candidates,
	})
}

type adminNodeSSHExclusiveSetReq struct {
	Enabled        bool     `json:"enabled"`
	ExclusiveUsers []string `json:"exclusive_users"`
}

func (s *Server) handleAdminNodeSSHExclusiveSet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req adminNodeSSHExclusiveSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	if err := s.store.ReplaceNodeExclusiveUsers(c.Request.Context(), nodeID, req.Enabled, req.ExclusiveUsers, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 更新节点独享策略，立即刷新 SSH 策略缓存", operator),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"node_id":         nodeID,
		"enabled":         req.Enabled,
		"exclusive_users": uniqTrim(req.ExclusiveUsers),
	})
}

func (s *Server) handleAdminNodeViewAccessGet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := s.store.ListNodeViewACL(c.Request.Context(), nodeID, 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	powerUsers, err := s.store.ListPowerUsers(c.Request.Context(), 5000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidates := make([]string, 0, len(powerUsers))
	for _, p := range powerUsers {
		if p.CanViewNodes {
			candidates = append(candidates, p.Username)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":             nodeID,
		"restricted":          len(allowed) > 0,
		"allowed_power_users": allowed,
		"candidates":          candidates,
	})
}

type adminNodeViewAccessSetReq struct {
	Restricted        bool     `json:"restricted"`
	AllowedPowerUsers []string `json:"allowed_power_users"`
}

func (s *Server) handleAdminNodeViewAccessSet(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req adminNodeViewAccessSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	operator := strings.TrimSpace(c.GetString("auth_user"))
	if operator == "" {
		operator = "admin"
	}
	allowed := []string{}
	if req.Restricted {
		allowed = req.AllowedPowerUsers
	}
	if err := s.store.ReplaceNodeViewACL(c.Request.Context(), nodeID, allowed, operator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":                  true,
		"node_id":             nodeID,
		"restricted":          len(allowed) > 0,
		"allowed_power_users": uniqTrim(allowed),
	})
}

func (s *Server) handleAdminNodeSSHUserPlatform(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	localUsername := strings.TrimSpace(c.Param("username"))
	if !s.ensureNodeReadable(c, nodeID) {
		return
	}
	if nodeID == "" || localUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
		return
	}
	billing, mapped, err := s.store.ResolveBillingUsername(c.Request.Context(), nodeID, localUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidate := localUsername
	if mapped && strings.TrimSpace(billing) != "" {
		candidate = strings.TrimSpace(billing)
	}
	exists, err := s.store.IsPlatformUserExists(c.Request.Context(), candidate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	platform := ""
	realName := ""
	msg := "未在平台开通"
	if exists {
		platform = candidate
		if acc, err := s.store.GetUserAccountByUsername(c.Request.Context(), candidate); err == nil {
			realName = strings.TrimSpace(acc.RealName)
		}
		if mapped {
			msg = "已映射到平台账号"
		} else {
			msg = "节点账号与平台账号同名"
		}
	} else if mapped {
		msg = "映射存在，但平台账号不存在"
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":           nodeID,
		"local_username":    localUsername,
		"mapping_exists":    mapped,
		"mapped_username":   billing,
		"platform_exists":   exists,
		"platform_username": platform,
		"real_name":         realName,
		"message":           msg,
	})
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

func (s *Server) handleAdminNodeKillAllUserProcesses(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
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
		Type:   "kill_all_user_processes",
		Reason: fmt.Sprintf("管理员 %s 发起：清理节点全部用户进程", operator),
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":           true,
		"node_id":      nodeID,
		"message":      "已下发清理全部用户进程指令，节点会在约 1 秒内执行",
		"requested_by": operator,
		"requested_at": time.Now().UTC().Format(time.RFC3339),
	})
}

type adminNodeDisconnectUserReq struct {
	LocalUsername string `json:"local_username"`
}

func (s *Server) handleAdminNodeDisconnectSSHUser(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req adminNodeDisconnectUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	localUsername := strings.TrimSpace(req.LocalUsername)
	if localUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local_username 不能为空"})
		return
	}

	exempted, err := s.store.IsExempted(c.Request.Context(), nodeID, localUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exempted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该账号属于 SSH 豁免账号，已跳过强制下线"})
		return
	}

	operator := strings.TrimSpace(fmt.Sprintf("%v", c.GetString("auth_user")))
	if operator == "" {
		operator = "admin"
	}
	s.enqueueNodeAction(nodeID, Action{
		Type:     "kick_ssh_user",
		Username: localUsername,
		Reason:   fmt.Sprintf("管理员 %s 发起：强制下线 SSH 账号 %s", operator, localUsername),
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"node_id":        nodeID,
		"local_username": localUsername,
		"message":        "已下发强制下线指令，节点会在约 1 秒内执行",
		"requested_by":   operator,
		"requested_at":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminNodeSyncNow(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("id"))
	if !s.ensureNodeReadable(c, nodeID) {
		return
	}
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	if _, err := s.store.GetNodeStatus(c.Request.Context(), nodeID); err != nil {
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
		Type:   "force_sync",
		Reason: fmt.Sprintf("管理员 %s 发起：立即同步节点状态", operator),
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":           true,
		"node_id":      nodeID,
		"message":      "已下发立即同步指令，节点会在约 1 秒内执行",
		"requested_by": operator,
		"requested_at": time.Now().UTC().Format(time.RFC3339),
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

type adminMailSendReq struct {
	AllUsers  bool     `json:"all_users"`
	Usernames []string `json:"usernames"`
	Subject   string   `json:"subject"`
	Body      string   `json:"body"`
}

type adminMailSendResult struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
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

func (s *Server) handleAdminMailSend(c *gin.Context) {
	var req adminMailSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.Subject == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject 不能为空"})
		return
	}
	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body 不能为空"})
		return
	}
	if !req.AllUsers && len(req.Usernames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择收件用户，或勾选发送给全部用户"})
		return
	}

	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	usernames := req.Usernames
	if req.AllUsers {
		usernames = nil
	}
	recipients, err := s.store.ListUserMailRecipients(c.Request.Context(), usernames, 20000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(recipients) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可发送的收件用户（请检查目标用户是否存在且填写了邮箱）"})
		return
	}

	results := make([]adminMailSendResult, 0, len(recipients))
	okCount := 0
	failCount := 0
	for _, u := range recipients {
		to := strings.TrimSpace(u.Email)
		if to == "" {
			failCount++
			results = append(results, adminMailSendResult{
				Username: u.Username,
				Email:    to,
				Success:  false,
				Error:    "该用户未配置邮箱",
			})
			continue
		}
		body := req.Body
		body = strings.ReplaceAll(body, "{{username}}", u.Username)
		body = strings.ReplaceAll(body, "{{real_name}}", u.RealName)
		if err := sendPlainTextMail(settings, to, req.Subject, body); err != nil {
			failCount++
			results = append(results, adminMailSendResult{
				Username: u.Username,
				Email:    to,
				Success:  false,
				Error:    err.Error(),
			})
			continue
		}
		okCount++
		results = append(results, adminMailSendResult{
			Username: u.Username,
			Email:    to,
			Success:  true,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      failCount == 0,
		"total":   len(recipients),
		"success": okCount,
		"failed":  failCount,
		"results": results,
	})
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

type haSummary struct {
	UsersCount          int64  `json:"users_count"`
	UserAccountsCount   int64  `json:"user_accounts_count"`
	NodeAccountsCount   int64  `json:"node_accounts_count"`
	WhitelistCount      int64  `json:"whitelist_count"`
	BlacklistCount      int64  `json:"blacklist_count"`
	ExemptionsCount     int64  `json:"exemptions_count"`
	UsageRecordsCount   int64  `json:"usage_records_count"`
	PendingRequests     int64  `json:"pending_requests_count"`
	ProfilePendingCount int64  `json:"profile_pending_count"`
	LatestUsageAt       string `json:"latest_usage_at"`
	LatestNodeSeenAt    string `json:"latest_node_seen_at"`
	Digest              string `json:"digest"`
}

type haNodeStatus struct {
	Enabled    bool      `json:"enabled"`
	Node       string    `json:"node"`
	Role       string    `json:"role"`
	ListenAddr string    `json:"listen_addr"`
	CheckedAt  time.Time `json:"checked_at"`
	Summary    haSummary `json:"summary"`
}

func (s *Server) queryCount(ctx context.Context, q string) int64 {
	var n int64
	if err := s.store.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0
	}
	return n
}

func (s *Server) queryMaxTime(ctx context.Context, q string) string {
	var t sql.NullTime
	if err := s.store.db.QueryRowContext(ctx, q).Scan(&t); err != nil {
		return ""
	}
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func (s *Server) buildHASummary(ctx context.Context) haSummary {
	out := haSummary{
		UsersCount:          s.queryCount(ctx, `SELECT COUNT(1) FROM users`),
		UserAccountsCount:   s.queryCount(ctx, `SELECT COUNT(1) FROM user_accounts`),
		NodeAccountsCount:   s.queryCount(ctx, `SELECT COUNT(1) FROM user_node_accounts`),
		WhitelistCount:      s.queryCount(ctx, `SELECT COUNT(1) FROM ssh_whitelist`),
		BlacklistCount:      s.queryCount(ctx, `SELECT COUNT(1) FROM ssh_blacklist`),
		ExemptionsCount:     s.queryCount(ctx, `SELECT COUNT(1) FROM ssh_exemptions`),
		UsageRecordsCount:   s.queryCount(ctx, `SELECT COUNT(1) FROM usage_records`),
		PendingRequests:     s.queryCount(ctx, `SELECT COUNT(1) FROM user_requests WHERE status='pending'`),
		ProfilePendingCount: s.queryCount(ctx, `SELECT COUNT(1) FROM profile_change_requests WHERE status='pending'`),
		LatestUsageAt:       s.queryMaxTime(ctx, `SELECT MAX(timestamp) FROM usage_records`),
		LatestNodeSeenAt:    s.queryMaxTime(ctx, `SELECT MAX(last_seen_at) FROM nodes`),
	}
	raw := fmt.Sprintf("%d|%d|%d|%d|%d|%d|%d|%d|%d|%s|%s",
		out.UsersCount,
		out.UserAccountsCount,
		out.NodeAccountsCount,
		out.WhitelistCount,
		out.BlacklistCount,
		out.ExemptionsCount,
		out.UsageRecordsCount,
		out.PendingRequests,
		out.ProfilePendingCount,
		out.LatestUsageAt,
		out.LatestNodeSeenAt,
	)
	sum := sha256.Sum256([]byte(raw))
	out.Digest = hex.EncodeToString(sum[:])
	return out
}

func (s *Server) localHAStatus(ctx context.Context) haNodeStatus {
	return haNodeStatus{
		Enabled:    s.cfg.HAEnabled,
		Node:       strings.TrimSpace(s.cfg.HANode),
		Role:       strings.TrimSpace(strings.ToLower(s.cfg.HARole)),
		ListenAddr: strings.TrimSpace(s.cfg.ListenAddr),
		CheckedAt:  time.Now().UTC(),
		Summary:    s.buildHASummary(ctx),
	}
}

func (s *Server) handleHAStatus(c *gin.Context) {
	// 主备之间状态查询：启用 token 时必须校验
	want := strings.TrimSpace(s.cfg.HAToken)
	if want != "" {
		got := strings.TrimSpace(c.GetHeader("X-HA-Token"))
		if got == "" || got != want {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": s.localHAStatus(c.Request.Context())})
}

func (s *Server) handleAdminHAStatus(c *gin.Context) {
	local := s.localHAStatus(c.Request.Context())
	peerURL := strings.TrimSpace(s.cfg.HAPeerURL)
	if !s.cfg.HAEnabled || peerURL == "" {
		c.JSON(http.StatusOK, gin.H{
			"enabled": s.cfg.HAEnabled,
			"local":   local,
			"peer":    nil,
			"in_sync": false,
			"message": "未配置容灾对端",
		})
		return
	}

	peerEndpoint := strings.TrimRight(peerURL, "/") + "/api/ha/status"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, peerEndpoint, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": s.cfg.HAEnabled,
			"local":   local,
			"peer":    gin.H{"reachable": false, "error": err.Error(), "url": peerURL},
			"in_sync": false,
		})
		return
	}
	if tok := strings.TrimSpace(s.cfg.HAToken); tok != "" {
		req.Header.Set("X-HA-Token", tok)
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": s.cfg.HAEnabled,
			"local":   local,
			"peer":    gin.H{"reachable": false, "error": err.Error(), "url": peerURL},
			"in_sync": false,
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		c.JSON(http.StatusOK, gin.H{
			"enabled": s.cfg.HAEnabled,
			"local":   local,
			"peer":    gin.H{"reachable": false, "error": fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))), "url": peerURL},
			"in_sync": false,
		})
		return
	}
	var peerWrap struct {
		Status haNodeStatus `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peerWrap); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": s.cfg.HAEnabled,
			"local":   local,
			"peer":    gin.H{"reachable": false, "error": "解析对端状态失败: " + err.Error(), "url": peerURL},
			"in_sync": false,
		})
		return
	}
	inSync := peerWrap.Status.Summary.Digest != "" && peerWrap.Status.Summary.Digest == local.Summary.Digest
	c.JSON(http.StatusOK, gin.H{
		"enabled":  s.cfg.HAEnabled,
		"local":    local,
		"peer":     gin.H{"reachable": true, "url": peerURL, "status": peerWrap.Status},
		"in_sync":  inSync,
		"note":     "建议主备控制器连接同一个 PostgreSQL 数据库以实现自动同步与恢复后一致性",
		"checked":  time.Now().UTC().Format(time.RFC3339),
		"peer_url": peerURL,
	})
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
	intervalSeconds := s.cfg.SampleIntervalSeconds
	if data.IntervalSeconds > 0 && data.IntervalSeconds <= 600 {
		intervalSeconds = data.IntervalSeconds
	}
	intervalMinutes := float64(intervalSeconds) / 60.0
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}

	type billingAgg struct {
		cost   float64
		locals map[string]struct{} // local_username set
	}
	billingAggs := make(map[string]*billingAgg)
	usageRecords := 0
	gpuProcCount := 0
	cpuProcCount := 0
	costTotal := 0.0
	cpuPercentSum := 0.0
	memoryMBSum := 0.0

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
		nodePriceOverride, nodeModelPriceOverrides, err := s.store.GetNodePricePolicyTx(ctx, tx, data.NodeID)
		if err != nil {
			return err
		}
		nodeModelPriceRows := make([]PriceRow, 0, len(nodeModelPriceOverrides))
		for model, price := range nodeModelPriceOverrides {
			nodeModelPriceRows = append(nodeModelPriceRows, PriceRow{Model: model, Price: price})
		}
		nodeModelPriceIndex := NewPriceIndex(nodeModelPriceRows)
		nodePointsPolicy, err := s.store.GetNodePointsInterceptPolicyTx(ctx, tx, data.NodeID)
		if err != nil {
			return err
		}
		nodeThrottleThreshold, nodeLimitedCPUQuota, nodeBlockedCPUQuota, nodeOverdraftMemoryLimitGB := s.resolveNodePointsInterceptPolicyValues(nodePointsPolicy)
		monthlyCfg, err := s.store.LoadMonthlyPointsConfigTx(ctx, tx)
		if err != nil {
			return err
		}
		// 语义约定：积分拦截开关“开启” => 正常扣分与限速；“关闭” => 不扣分且不限速。
		nodePointsBillingEnabled := nodePointsPolicy.Enabled

		// 同一台节点的映射/豁免在一次上报内复用，避免对每个进程重复查库。
		resolveCache := make(map[string]string) // local_username -> billing_username（未绑定时为自身）
		exemptCache := make(map[string]bool)    // local_username -> 是否 SSH 豁免
		isExemptLocal := func(localUsername string) (bool, error) {
			localUsername = strings.TrimSpace(localUsername)
			if localUsername == "" {
				return false, nil
			}
			if v, ok := exemptCache[localUsername]; ok {
				return v, nil
			}
			exempted, err := s.store.IsExempted(ctx, data.NodeID, localUsername)
			if err != nil {
				return false, err
			}
			exemptCache[localUsername] = exempted
			return exempted, nil
		}
		miningEvidenceByUser := make(map[string][]map[string]any)
		billedLocalUsers := make(map[string]struct{})

		for _, proc := range data.Users {
			localUsername := strings.TrimSpace(proc.Username)
			if localUsername == "" {
				continue
			}
			cmdRaw := strings.TrimSpace(proc.Command)
			if indicators := detectMiningIndicators(cmdRaw); len(indicators) > 0 {
				if len(miningEvidenceByUser[localUsername]) < 5 {
					miningEvidenceByUser[localUsername] = append(miningEvidenceByUser[localUsername], map[string]any{
						"pid":            proc.PID,
						"command":        cmdRaw,
						"indicators":     indicators,
						"cpu_percent":    round4(proc.CPUPercent),
						"gpu_card_count": len(proc.GPUUsage),
					})
				}
			}
			exempted, err := isExemptLocal(localUsername)
			if err != nil {
				return err
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
				gpuCost = CalculateProcessCostWithPolicy(proc, nodeModelPriceIndex, nodePriceOverride, priceIndex, s.cfg.DefaultPricePerMinute)
			}
			proc.Command = cmdRaw
			if len(proc.Command) > 256 {
				proc.Command = proc.Command[:256]
			}
			cpuCost := (proc.CPUPercent / 100.0) * cpuPricePerCoreMinute * intervalMinutes
			cost := round4(gpuCost + cpuCost)
			if !nodePointsBillingEnabled {
				// 积分拦截关闭：保留使用记录，但不计入积分消耗。
				cost = 0
			} else if exempted {
				// SSH 豁免账号放行时仅记录使用，不参与积分扣减和积分限速动作。
				cost = 0
			}

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
			billedLocalUsers[localUsername] = struct{}{}
			usageRecords++
			costTotal += cost
			cpuPercentSum += proc.CPUPercent
			memoryMBSum += proc.MemoryMB
			if len(proc.GPUUsage) > 0 {
				gpuProcCount++
			} else {
				cpuProcCount++
			}

			if !exempted {
				b := billingAggs[billingUsername]
				if b == nil {
					b = &billingAgg{locals: make(map[string]struct{})}
					billingAggs[billingUsername] = b
				}
				b.cost += cost
				b.locals[localUsername] = struct{}{}
			}
		}
		if len(miningEvidenceByUser) > 0 {
			relatedUsers := make([]string, 0, len(miningEvidenceByUser))
			summary := make([]string, 0, len(miningEvidenceByUser))
			detailsUsers := make([]map[string]any, 0, len(miningEvidenceByUser))
			for username, hits := range miningEvidenceByUser {
				relatedUsers = append(relatedUsers, username)
				summary = append(summary, fmt.Sprintf("%s(%d 进程)", username, len(hits)))
				detailsUsers = append(detailsUsers, map[string]any{
					"username":         username,
					"suspicious_count": len(hits),
					"samples":          hits,
				})
			}
			sort.Strings(relatedUsers)
			sort.Strings(summary)
			sort.Slice(detailsUsers, func(i, j int) bool {
				li := strings.TrimSpace(fmt.Sprintf("%v", detailsUsers[i]["username"]))
				lj := strings.TrimSpace(fmt.Sprintf("%v", detailsUsers[j]["username"]))
				return li < lj
			})
			reason := "检测到疑似挖矿进程特征：" + strings.Join(summary, "，")
			if _, err := s.store.InsertNodeSecurityEventTx(
				ctx,
				tx,
				data.ReportID,
				data.NodeID,
				"suspected_mining",
				"critical",
				reason,
				relatedUsers,
				map[string]any{
					"node_id":               data.NodeID,
					"report_ts":             reportTS.UTC().Format(time.RFC3339),
					"related_usernames":     relatedUsers,
					"judgement":             "疑似挖矿",
					"suspicious_user_count": len(relatedUsers),
					"users":                 detailsUsers,
					"security_signal_input": "process.command",
				},
				2*time.Minute,
			); err != nil {
				return err
			}
		}

		// 节点总体 CPU 高负载告警（默认阈值 95%）：用于追踪疑似恶意账号并形成审计日志。
		type userCPUStat struct {
			username   string
			cpuPercent float64
		}
		cpuByUser := map[string]float64{}
		for _, proc := range data.Users {
			user := strings.TrimSpace(proc.Username)
			if user == "" {
				continue
			}
			cpuByUser[user] += proc.CPUPercent
		}
		topUsers := make([]userCPUStat, 0, len(cpuByUser))
		for user, sum := range cpuByUser {
			topUsers = append(topUsers, userCPUStat{username: user, cpuPercent: round4(sum)})
		}
		sort.Slice(topUsers, func(i, j int) bool {
			if topUsers[i].cpuPercent == topUsers[j].cpuPercent {
				return topUsers[i].username < topUsers[j].username
			}
			return topUsers[i].cpuPercent > topUsers[j].cpuPercent
		})
		topN := 10
		if len(topUsers) < topN {
			topN = len(topUsers)
		}
		topPayload := make([]map[string]any, 0, topN)
		relatedUsers := make([]string, 0, topN)
		for i := 0; i < topN; i++ {
			topPayload = append(topPayload, map[string]any{
				"username":    topUsers[i].username,
				"cpu_percent": topUsers[i].cpuPercent,
			})
			relatedUsers = append(relatedUsers, topUsers[i].username)
		}
		nodeCPUCount := data.CPUCount
		if nodeCPUCount <= 0 {
			nodeCPUCount = 1
		}
		totalCPUPercent := float64(nodeCPUCount) * 100.0
		usageRatio := 0.0
		if totalCPUPercent > 0 {
			usageRatio = cpuPercentSum / totalCPUPercent
		}
		if usageRatio >= 0.95 {
			reason := fmt.Sprintf("节点 CPU 总负载 %.2f%%（阈值 95%%）", usageRatio*100.0)
			_, err := s.store.InsertNodeSecurityEventTx(
				ctx,
				tx,
				data.ReportID,
				data.NodeID,
				"high_cpu_load",
				"warning",
				reason,
				relatedUsers,
				map[string]any{
					"node_id":                data.NodeID,
					"report_ts":              reportTS.UTC().Format(time.RFC3339),
					"cpu_percent_sum":        round4(cpuPercentSum),
					"node_cpu_count":         nodeCPUCount,
					"threshold_percent":      95.0,
					"node_capacity_percent":  round4(totalCPUPercent),
					"usage_ratio_percent":    round4(usageRatio * 100.0),
					"top_user_cpu_breakdown": topPayload,
				},
				5*time.Minute,
			)
			if err != nil {
				return err
			}
		}
		if err := s.recordNodeSecuritySignalsTx(ctx, tx, data, reportTS); err != nil {
			return err
		}

		for billingUsername, b := range billingAggs {
			if !nodePointsBillingEnabled {
				// 积分拦截关闭时：不扣减积分，也不触发欠费状态动作。
				continue
			}
			res, err := s.store.DeductBalanceOnNodeTx(ctx, tx, billingUsername, data.NodeID, b.cost, now, s.cfg)
			if err != nil {
				return err
			}
			effectiveBalance := res.EffectiveBalance
			prevOverdraftExceeded := isOverdraftExceeded(res.PrevEffectiveBalance, monthlyCfg.MaxOverdraftLimit)
			nowOverdraftExceeded := isOverdraftExceeded(effectiveBalance, monthlyCfg.MaxOverdraftLimit)

			// 注意：扣费与余额状态以“计费账号”为准；但下发动作必须针对“节点本地账号”，否则 Agent 无法生效。
			for localUsername := range b.locals {
				uLocal := res.User
				uLocal.Username = localUsername
				uLocal.Balance = effectiveBalance
				actions = append(actions, DecideActions(res.PrevStatus, uLocal, s.cfg.WarningThreshold, s.cfg.LimitedThreshold, monthlyCfg.MaxOverdraftLimit)...)
				gpuReason := fmt.Sprintf("余额 %.2f 未超过欠费阈值 %.2f，恢复 GPU 可见性", effectiveBalance, killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit))
				if nowOverdraftExceeded {
					gpuReason = fmt.Sprintf("余额 %.2f 已超过欠费上限 %.2f（欠费阈值 %.2f），加入欠费 GPU 限制组并隐藏 GPU", effectiveBalance, normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit), killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit))
				}
				if gpuAction, ok := s.nextGPUAccessAction(data.NodeID, localUsername, nowOverdraftExceeded, gpuReason, false); ok {
					actions = append(actions, gpuAction)
				}
				targetMemoryGB := 0.0
				memoryReason := fmt.Sprintf(
					"余额 %.2f 未超过每月欠费上限 %.2f，解除内存限额",
					effectiveBalance,
					normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit),
				)
				if nowOverdraftExceeded && nodeOverdraftMemoryLimitGB > 0 {
					targetMemoryGB = nodeOverdraftMemoryLimitGB
					memoryReason = fmt.Sprintf(
						"余额 %.2f 已超过每月欠费上限 %.2f，内存上限设为 %.2f GB",
						effectiveBalance,
						normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit),
						targetMemoryGB,
					)
				}
				if memoryAction, ok := s.nextMemoryLimitAction(data.NodeID, localUsername, targetMemoryGB, memoryReason, false); ok {
					actions = append(actions, memoryAction)
				}

				if s.cfg.EnableCPUControl {
					switch {
					case effectiveBalance <= 0:
						if quotaAction, ok := s.nextCPUQuotaAction(
							data.NodeID,
							localUsername,
							nodeBlockedCPUQuota,
							fmt.Sprintf("余额 %.2f<=0，按节点策略执行欠费强限速（%.2f%%）", effectiveBalance, nodeBlockedCPUQuota),
							false,
						); ok {
							actions = append(actions, quotaAction)
						}
					case effectiveBalance <= nodeThrottleThreshold:
						if quotaAction, ok := s.nextCPUQuotaAction(
							data.NodeID,
							localUsername,
							nodeLimitedCPUQuota,
							fmt.Sprintf("余额 %.2f<=阈值 %.2f，按节点策略执行限速（%.2f%%）", effectiveBalance, nodeThrottleThreshold, nodeLimitedCPUQuota),
							false,
						); ok {
							actions = append(actions, quotaAction)
						}
					default:
						if quotaAction, ok := s.nextCPUQuotaAction(data.NodeID, localUsername, 0, "余额已恢复，解除 CPU 限制", false); ok {
							actions = append(actions, quotaAction)
						}
					}
				}
				if nowOverdraftExceeded && !prevOverdraftExceeded {
					actions = append(actions, Action{
						Type:     "kill_all_processes",
						Username: localUsername,
						Reason: fmt.Sprintf(
							"余额 %.2f 首次超过每月欠费上限 %.2f（阈值 %.2f），执行一次性清理全部进程",
							effectiveBalance,
							normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit),
							killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit),
						),
					})
				}
			}
		}

		// 兜底同步：对“仅在线 SSH、当前无计费进程”的账号，也按实时余额下发 GPU/CPU 限制动作。
		// 避免出现“积分已负值但状态/限速未及时更新”的窗口期。
		type balanceSnapshot struct {
			storedStatus     string
			effectiveBalance float64
		}
		balanceByBilling := make(map[string]balanceSnapshot)
		loadBalance := func(billingUsername string) (balanceSnapshot, error) {
			if v, ok := balanceByBilling[billingUsername]; ok {
				return v, nil
			}
			u, err := s.store.GetUserTx(ctx, tx, billingUsername)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					v := balanceSnapshot{storedStatus: "normal", effectiveBalance: s.cfg.DefaultBalance}
					balanceByBilling[billingUsername] = v
					return v, nil
				}
				return balanceSnapshot{}, err
			}
			v := balanceSnapshot{
				storedStatus:     u.Status,
				effectiveBalance: u.Balance + u.CarryoverBalance,
			}
			balanceByBilling[billingUsername] = v
			return v, nil
		}
		for _, sshUser := range data.SSHUsers {
			localUsername := strings.TrimSpace(sshUser)
			if localUsername == "" || strings.EqualFold(localUsername, "root") {
				continue
			}
			exempted, err := isExemptLocal(localUsername)
			if err != nil {
				return err
			}
			if exempted {
				// 豁免账号不参与积分余额驱动的限速/欠费动作。
				continue
			}
			if _, alreadyHandled := billedLocalUsers[localUsername]; alreadyHandled {
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
			if billingUsername == "" {
				continue
			}

			bal, err := loadBalance(billingUsername)
			if err != nil {
				return err
			}
			overdraftExceeded := isOverdraftExceeded(bal.effectiveBalance, monthlyCfg.MaxOverdraftLimit)
			gpuReason := fmt.Sprintf("SSH 在线账号余额同步：余额 %.2f 未超过欠费阈值 %.2f，恢复 GPU 可见性", bal.effectiveBalance, killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit))
			if overdraftExceeded {
				gpuReason = fmt.Sprintf("SSH 在线账号余额同步：余额 %.2f 已超过每月欠费上限 %.2f（阈值 %.2f），加入欠费 GPU 限制组并隐藏 GPU", bal.effectiveBalance, normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit), killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit))
			}
			if action, ok := s.nextGPUAccessAction(data.NodeID, localUsername, overdraftExceeded, gpuReason, false); ok {
				actions = append(actions, action)
			}
			targetMemoryGB := 0.0
			memoryReason := "SSH 在线账号余额同步：节点积分拦截关闭，解除内存限额"
			if !nodePointsBillingEnabled {
				targetMemoryGB = 0
			} else if overdraftExceeded && nodeOverdraftMemoryLimitGB > 0 {
				targetMemoryGB = nodeOverdraftMemoryLimitGB
				memoryReason = fmt.Sprintf(
					"SSH 在线账号余额同步：余额 %.2f 已超过每月欠费上限 %.2f，内存上限设为 %.2f GB",
					bal.effectiveBalance,
					normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit),
					targetMemoryGB,
				)
			} else {
				memoryReason = fmt.Sprintf(
					"SSH 在线账号余额同步：余额 %.2f 未超过每月欠费上限 %.2f，解除内存限额",
					bal.effectiveBalance,
					normalizeMonthlyMaxOverdraftLimit(monthlyCfg.MaxOverdraftLimit),
				)
			}
			if memoryAction, ok := s.nextMemoryLimitAction(data.NodeID, localUsername, targetMemoryGB, memoryReason, false); ok {
				actions = append(actions, memoryAction)
			}

			if s.cfg.EnableCPUControl {
				targetQuota := 0.0
				quotaReason := "SSH 在线账号余额同步：状态正常，解除 CPU 限制"
				switch {
				case !nodePointsBillingEnabled:
					targetQuota = 0
					quotaReason = "SSH 在线账号余额同步：节点积分拦截关闭，解除 CPU 限制"
				case bal.effectiveBalance <= 0:
					targetQuota = nodeBlockedCPUQuota
					quotaReason = fmt.Sprintf("SSH 在线账号余额同步：余额 %.2f<=0，执行欠费强限速（%.2f%%）", bal.effectiveBalance, nodeBlockedCPUQuota)
				case bal.effectiveBalance <= nodeThrottleThreshold:
					targetQuota = nodeLimitedCPUQuota
					quotaReason = fmt.Sprintf("SSH 在线账号余额同步：余额 %.2f<=阈值 %.2f，执行限速（%.2f%%）", bal.effectiveBalance, nodeThrottleThreshold, nodeLimitedCPUQuota)
				}
				if quotaAction, ok := s.nextCPUQuotaAction(data.NodeID, localUsername, targetQuota, quotaReason, false); ok {
					actions = append(actions, quotaAction)
				}
			}
		}

		// 更新节点状态（用于运维查看在线/上报情况）
		if err := s.store.UpsertNodeRuntimeSnapshotTx(
			ctx,
			tx,
			data.ReportID,
			data.NodeID,
			reportTS,
			round4(cpuPercentSum),
			round4(memoryMBSum),
			gpuProcCount,
			cpuProcCount,
			data.SSHUsers,
			round4(costTotal),
		); err != nil {
			return err
		}

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
			data.OSVersion,
			data.KernelVersion,
			data.NodeIP,
			data.NodeMAC,
			data.DiskTotalGB,
			data.DiskUsedGB,
			data.HomeTotalGB,
			data.HomeUsedGB,
			data.MntTotalGB,
			data.MntUsedGB,
			data.NetRxBytes,
			data.NetTxBytes,
			gpuProcCount,
			cpuProcCount,
			usageRecords,
			len(data.SSHUsers),
			data.DiskQuotaInstalled,
			data.DiskQuotaMounts,
			round4(costTotal),
		); err != nil {
			return err
		}
		if err := s.store.ReplaceNodeLocalUsersTx(ctx, tx, data.NodeID, data.LocalUsers); err != nil {
			return err
		}
		if err := s.store.ReplaceNodeUserDiskQuotasTx(ctx, tx, data.NodeID, data.UserDiskQuotas); err != nil {
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

func detectMiningIndicators(command string) []string {
	cmd := strings.ToLower(strings.TrimSpace(command))
	if cmd == "" {
		return nil
	}
	rules := []struct {
		Needle string
		Label  string
	}{
		{Needle: "xmrig", Label: "命中 xmrig 特征"},
		{Needle: "cpuminer", Label: "命中 cpuminer 特征"},
		{Needle: "minerd", Label: "命中 minerd 特征"},
		{Needle: "ethminer", Label: "命中 ethminer 特征"},
		{Needle: "phoenixminer", Label: "命中 phoenixminer 特征"},
		{Needle: "lolminer", Label: "命中 lolminer 特征"},
		{Needle: "nbminer", Label: "命中 nbminer 特征"},
		{Needle: "gminer", Label: "命中 gminer 特征"},
		{Needle: "teamredminer", Label: "命中 teamredminer 特征"},
		{Needle: "nanominer", Label: "命中 nanominer 特征"},
		{Needle: "t-rex", Label: "命中 t-rex 特征"},
		{Needle: "stratum+tcp", Label: "命中 stratum 协议特征"},
		{Needle: "stratum://", Label: "命中 stratum 协议特征"},
		{Needle: "nicehash", Label: "命中 nicehash 特征"},
		{Needle: "randomx", Label: "命中 randomx 挖矿算法特征"},
		{Needle: "ethash", Label: "命中 ethash 挖矿算法特征"},
		{Needle: "kawpow", Label: "命中 kawpow 挖矿算法特征"},
		{Needle: "cryptonight", Label: "命中 cryptonight 挖矿算法特征"},
	}
	set := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, rule := range rules {
		if !strings.Contains(cmd, rule.Needle) {
			continue
		}
		if _, ok := set[rule.Label]; ok {
			continue
		}
		set[rule.Label] = struct{}{}
		out = append(out, rule.Label)
	}
	sort.Strings(out)
	return out
}

func (s *Server) recordNodeSecuritySignalsTx(ctx context.Context, tx *sql.Tx, data MetricsData, reportTS time.Time) error {
	const sshFailThreshold5m = 20
	const signalClearHold = 10 * time.Minute
	if sig := data.SecuritySignals; sig != nil {
		sshFailedSpikeActive := sig.SSHFailedCount5m > sshFailThreshold5m
		if sshFailedSpikeActive && s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_failed_login_spike", true, reportTS, signalClearHold) {
			reason := fmt.Sprintf("5 分钟内 SSH 失败登录 %d 次（阈值 %d 次）", sig.SSHFailedCount5m, sshFailThreshold5m)
			if _, err := s.store.InsertNodeSecurityEventTx(
				ctx,
				tx,
				data.ReportID,
				data.NodeID,
				"ssh_failed_login_spike",
				"critical",
				reason,
				sig.SSHFailedUsernames,
				map[string]any{
					"node_id":               data.NodeID,
					"report_ts":             reportTS.UTC().Format(time.RFC3339),
					"window_minutes":        5,
					"failed_count":          sig.SSHFailedCount5m,
					"threshold":             sshFailThreshold5m,
					"failed_source_ips":     sig.SSHFailedSourceIPs,
					"failed_usernames":      sig.SSHFailedUsernames,
					"judgement":             "恶意登录失败峰值",
					"security_signal_input": "agent.security_signals",
				},
				0,
			); err != nil {
				return err
			}
		}
		if !sshFailedSpikeActive {
			s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_failed_login_spike", false, reportTS, signalClearHold)
		}

		sshBruteforceActive := sig.SSHBruteforceDetected
		if sshBruteforceActive && s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_bruteforce", true, reportTS, signalClearHold) {
			reason := "检测到 SSH 爆破行为"
			if len(sig.SSHBruteforceSources) > 0 {
				reason = fmt.Sprintf("检测到 SSH 爆破行为：来源 %s", strings.Join(sig.SSHBruteforceSources, ", "))
			}
			if _, err := s.store.InsertNodeSecurityEventTx(
				ctx,
				tx,
				data.ReportID,
				data.NodeID,
				"ssh_bruteforce",
				"critical",
				reason,
				sig.SSHFailedUsernames,
				map[string]any{
					"node_id":               data.NodeID,
					"report_ts":             reportTS.UTC().Format(time.RFC3339),
					"window_minutes":        5,
					"bruteforce_sources":    sig.SSHBruteforceSources,
					"failed_count":          sig.SSHFailedCount5m,
					"failed_source_ips":     sig.SSHFailedSourceIPs,
					"failed_usernames":      sig.SSHFailedUsernames,
					"judgement":             "SSH 爆破",
					"security_signal_input": "agent.security_signals",
				},
				0,
			); err != nil {
				return err
			}
		}
		if !sshBruteforceActive {
			s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_bruteforce", false, reportTS, signalClearHold)
		}

		portScanActive := sig.PortScanDetected
		if portScanActive && s.shouldEmitSecuritySignalEvent(data.NodeID, "abnormal_port_scan", true, reportTS, signalClearHold) {
			reason := "检测到异常端口扫描"
			if len(sig.PortScanSources) > 0 {
				reason = fmt.Sprintf("检测到异常端口扫描：来源 %s", strings.Join(sig.PortScanSources, ", "))
			}
			if _, err := s.store.InsertNodeSecurityEventTx(
				ctx,
				tx,
				data.ReportID,
				data.NodeID,
				"abnormal_port_scan",
				"critical",
				reason,
				nil,
				map[string]any{
					"node_id":               data.NodeID,
					"report_ts":             reportTS.UTC().Format(time.RFC3339),
					"scan_sources":          sig.PortScanSources,
					"scan_target_ports":     sig.PortScanTargetPorts,
					"judgement":             "异常端口扫描",
					"security_signal_input": "agent.security_signals",
				},
				0,
			); err != nil {
				return err
			}
		}
		if !portScanActive {
			s.shouldEmitSecuritySignalEvent(data.NodeID, "abnormal_port_scan", false, reportTS, signalClearHold)
		}
	} else {
		// agent 未上报安全信号时，按静默期尝试清理告警态，避免短暂缺失引起反复告警。
		s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_failed_login_spike", false, reportTS, signalClearHold)
		s.shouldEmitSecuritySignalEvent(data.NodeID, "ssh_bruteforce", false, reportTS, signalClearHold)
		s.shouldEmitSecuritySignalEvent(data.NodeID, "abnormal_port_scan", false, reportTS, signalClearHold)
	}

	riskyMounts := collectRiskyDiskMounts(data)
	if len(riskyMounts) == 0 {
		return nil
	}
	reasonParts := make([]string, 0, len(riskyMounts))
	detailsMounts := make([]map[string]any, 0, len(riskyMounts))
	for _, m := range riskyMounts {
		reasonParts = append(reasonParts, fmt.Sprintf("%s 使用率 %.2f%%（剩余 %.2fGB）", m.Name, m.UsedPercent, m.FreeGB))
		detailsMounts = append(detailsMounts, map[string]any{
			"name":         m.Name,
			"total_gb":     round4(m.TotalGB),
			"used_gb":      round4(m.UsedGB),
			"free_gb":      round4(m.FreeGB),
			"used_percent": round4(m.UsedPercent),
		})
	}
	relatedUsers := topHomeHeavyUsers(data.LocalUsers, 10)
	reason := "检测到磁盘打满风险：" + strings.Join(reasonParts, "；")
	_, err := s.store.InsertNodeSecurityEventTx(
		ctx,
		tx,
		data.ReportID,
		data.NodeID,
		"disk_full_risk",
		"critical",
		reason,
		relatedUsers,
		map[string]any{
			"node_id":               data.NodeID,
			"report_ts":             reportTS.UTC().Format(time.RFC3339),
			"risk_mounts":           detailsMounts,
			"related_usernames":     relatedUsers,
			"threshold_percent":     98.0,
			"threshold_free_gb":     1.0,
			"judgement":             "磁盘打满风险",
			"security_signal_input": "node_storage_usage",
		},
		10*time.Minute,
	)
	return err
}

type diskMountRisk struct {
	Name        string
	TotalGB     float64
	UsedGB      float64
	FreeGB      float64
	UsedPercent float64
}

func collectRiskyDiskMounts(data MetricsData) []diskMountRisk {
	checks := []struct {
		name  string
		total float64
		used  float64
	}{
		{name: "/", total: data.DiskTotalGB, used: data.DiskUsedGB},
		{name: "/home", total: data.HomeTotalGB, used: data.HomeUsedGB},
		{name: "/mnt", total: data.MntTotalGB, used: data.MntUsedGB},
	}
	out := make([]diskMountRisk, 0, len(checks))
	for _, x := range checks {
		if x.total <= 0 {
			continue
		}
		used := x.used
		if used < 0 {
			used = 0
		}
		if used > x.total {
			used = x.total
		}
		free := x.total - used
		if free < 0 {
			free = 0
		}
		usedPercent := 0.0
		if x.total > 0 {
			usedPercent = (used / x.total) * 100.0
		}
		if usedPercent >= 98.0 || free <= 1.0 {
			out = append(out, diskMountRisk{
				Name:        x.name,
				TotalGB:     x.total,
				UsedGB:      used,
				FreeGB:      free,
				UsedPercent: usedPercent,
			})
		}
	}
	return out
}

func topHomeHeavyUsers(localUsers []NodeLocalUser, limit int) []string {
	if limit <= 0 || len(localUsers) == 0 {
		return nil
	}
	type item struct {
		username string
		homeGB   float64
	}
	items := make([]item, 0, len(localUsers))
	for _, u := range localUsers {
		username := strings.TrimSpace(u.LocalUsername)
		if username == "" || u.HomeUsedGB <= 0 {
			continue
		}
		items = append(items, item{username: username, homeGB: u.HomeUsedGB})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].homeGB == items[j].homeGB {
			return items[i].username < items[j].username
		}
		return items[i].homeGB > items[j].homeGB
	})
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, x := range items {
		if len(out) >= limit {
			break
		}
		if _, ok := seen[x.username]; ok {
			continue
		}
		seen[x.username] = struct{}{}
		out = append(out, x.username)
	}
	return out
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
