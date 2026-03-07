package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleRegistryResolve(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	localUsername := strings.TrimSpace(c.Query("local_username"))
	if nodeID == "" || localUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
		return
	}

	ctx := c.Request.Context()
	exempted, err := s.store.IsExempted(ctx, nodeID, localUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exempted {
		c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": localUsername, "exempted": true})
		return
	}

	blacklisted, err := s.store.IsBlacklisted(ctx, nodeID, localUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if blacklisted {
		c.JSON(http.StatusOK, gin.H{"registered": false, "blacklisted": true})
		return
	}

	exclusiveEnabled, exclusiveBlockOtherSSH, err := s.store.GetNodeSSHExclusiveConfig(ctx, nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exclusiveEnabled && exclusiveBlockOtherSSH {
		exclusiveUser, err := s.store.IsNodeExclusiveUser(ctx, nodeID, localUsername)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if exclusiveUser {
			c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": localUsername, "exclusive_user": true})
			return
		}
		whitelisted, err := s.store.IsWhitelisted(ctx, nodeID, localUsername)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if whitelisted {
			c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": localUsername, "whitelisted": true, "exclusive_bypass": true})
			return
		}
		c.JSON(http.StatusOK, gin.H{"registered": false, "exclusive_denied": true})
		return
	}

	sshGuardEnabled, err := s.store.IsNodeSSHGuardEnabled(ctx, nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !sshGuardEnabled {
		// 关闭“未注册拦截”时，只保留黑名单/豁免判断；普通未注册账号放行。
		c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": localUsername, "guard_enabled": false})
		return
	}

	var billing string
	found := false
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		billing, found, txErr = s.store.ResolveBillingUsernameTx(ctx, tx, nodeID, localUsername)
		return txErr
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !found {
		tempBilling, tempFound, tempExpiresAt, err := s.store.ResolveTemporaryBindAccess(ctx, nodeID, localUsername, time.Now())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if tempFound {
			payload := gin.H{
				"registered":       true,
				"temporary_bind":   true,
				"billing_username": tempBilling,
			}
			if tempExpiresAt != nil {
				payload["temporary_expires_at"] = *tempExpiresAt
			}
			c.JSON(http.StatusOK, payload)
			return
		}
		whitelisted, err := s.store.IsWhitelisted(ctx, nodeID, localUsername)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !whitelisted {
			c.JSON(http.StatusOK, gin.H{"registered": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": localUsername, "whitelisted": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"registered": true, "billing_username": billing})
}

type registryBindClaimReq struct {
	Token         string `json:"token"`
	NodeID        string `json:"node_id"`
	LocalUsername string `json:"local_username"`
}

func (s *Server) handleRegistryBindClaim(c *gin.Context) {
	var req registryBindClaimReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	if req.Token == "" || req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token/node_id/local_username 不能为空"})
		return
	}

	policy, err := s.store.GetNodeBindSecurityPolicy(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	var challenge UserNodeBindChallenge
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var txErr error
		challenge, txErr = s.store.ClaimUserBindChallengeTx(
			c.Request.Context(),
			tx,
			req.Token,
			req.NodeID,
			req.LocalUsername,
			now,
		)
		return txErr
	}); err != nil {
		switch {
		case errors.Is(err, errNodeBindChallengeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "challenge 不存在或已失效", "reason": "challenge_not_found"})
			return
		case errors.Is(err, errNodeBindChallengeMismatch):
			c.JSON(http.StatusForbidden, gin.H{"error": "challenge 与当前节点账号不匹配", "reason": "challenge_mismatch"})
			return
		case errors.Is(err, errNodeBindChallengeExpired):
			s.sweepNodeBindFailures(c.Request.Context(), req.NodeID)
			c.JSON(http.StatusGone, gin.H{"error": "challenge 已过期，请在网页重新申请", "reason": "challenge_expired"})
			return
		case errors.Is(err, errNodeBindAlreadyBound):
			c.JSON(http.StatusConflict, gin.H{"error": "该节点账号已绑定到当前平台账号", "reason": "mapping_exists_same_user"})
			return
		}
		var conflictErr *NodeAccountOwnershipConflictError
		if errors.As(err, &conflictErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "节点账号已被其他平台账号绑定，禁止换绑。请先走解绑审批流程。",
				"reason":         "mapping_exists_other_user",
				"node_id":        conflictErr.NodeID,
				"local_username": conflictErr.LocalUsername,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	successReason := fmt.Sprintf("绑定 challenge 校验成功：billing=%s node=%s local=%s", challenge.BillingUsername, challenge.NodeID, challenge.LocalUsername)
	s.enqueueCPUQuotaAction(challenge.NodeID, challenge.LocalUsername, 0, successReason+"；解除临时 CPU 限速", true)
	if memoryAction, ok := s.nextMemoryLimitAction(challenge.NodeID, challenge.LocalUsername, 0, successReason+"；解除临时内存限额", true); ok {
		s.enqueueNodeAction(challenge.NodeID, memoryAction)
	}
	if policy.TrialGPUBlocked {
		s.enqueueNodeAction(challenge.NodeID, Action{
			Type:     "unblock_user",
			Username: challenge.LocalUsername,
			Reason:   successReason + "；解除临时 GPU 隐藏",
		})
	}
	s.enqueueNodeAction(challenge.NodeID, Action{
		Type:   "force_sync",
		Reason: successReason + "；刷新 SSH 校验缓存",
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"request_id":       challenge.RequestID,
		"challenge_id":     challenge.ChallengeID,
		"billing_username": challenge.BillingUsername,
		"node_id":          challenge.NodeID,
		"local_username":   challenge.LocalUsername,
		"status":           challenge.Status,
		"message":          "绑定校验成功，映射已生效",
	})
}

// handleRegistryNodeGuardState 返回节点当前拦截策略开关，供节点端缓存策略参考。
func (s *Server) handleRegistryNodeGuardState(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	guardEnabled, err := s.store.IsNodeSSHGuardEnabled(c.Request.Context(), nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exclusiveEnabled, exclusiveBlockOtherSSH, err := s.store.GetNodeSSHExclusiveConfig(c.Request.Context(), nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":                   nodeID,
		"guard_enabled":             guardEnabled,
		"exclusive_enabled":         (exclusiveEnabled && exclusiveBlockOtherSSH),
		"exclusive_config_enabled":  exclusiveEnabled,
		"exclusive_block_other_ssh": exclusiveBlockOtherSSH,
	})
}

// handleRegistryNodeUsersTxt 返回该节点已登记的本地用户名列表（每行一个），用于 PAM/SSH 校验缓存同步。
func (s *Server) handleRegistryNodeUsersTxt(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	users, err := s.store.ListAllowedLocalUsersByNode(c.Request.Context(), nodeID, 200000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	if len(users) == 0 {
		c.String(http.StatusOK, "")
		return
	}
	c.String(http.StatusOK, strings.Join(users, "\n")+"\n")
}

// handleRegistryNodeBlockedUsersTxt 返回该节点拒绝登录的本地用户名列表（每行一个）。
func (s *Server) handleRegistryNodeBlockedUsersTxt(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	users, err := s.store.ListDeniedLocalUsersByNode(c.Request.Context(), nodeID, 200000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	if len(users) == 0 {
		c.String(http.StatusOK, "")
		return
	}
	c.String(http.StatusOK, strings.Join(users, "\n")+"\n")
}

// handleRegistryNodeExemptUsersTxt 返回该节点 SSH 豁免本地用户名列表（每行一个）。
func (s *Server) handleRegistryNodeExemptUsersTxt(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id 不能为空"})
		return
	}
	users, err := s.store.ListExemptLocalUsersByNode(c.Request.Context(), nodeID, 200000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	if len(users) == 0 {
		c.String(http.StatusOK, "")
		return
	}
	c.String(http.StatusOK, strings.Join(users, "\n")+"\n")
}

type bindRequestsCreateReq struct {
	BillingUsername string `json:"billing_username"`
	Items           []struct {
		NodeID        string `json:"node_id"`
		LocalUsername string `json:"local_username"`
	} `json:"items"`
	Message string `json:"message"`
}

func (s *Server) handleUserBindRequestsCreate(c *gin.Context) {
	authBilling := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req bindRequestsCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = authBilling
	req.Message = strings.TrimSpace(req.Message)
	if req.BillingUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing_username 不能为空"})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items 不能为空"})
		return
	}
	if len(req.Items) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items 过多（最大 200）"})
		return
	}

	ctx := c.Request.Context()
	var ids []int
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		for _, it := range req.Items {
			nodeID := strings.TrimSpace(it.NodeID)
			localUsername := strings.TrimSpace(it.LocalUsername)
			if nodeID == "" || localUsername == "" {
				return strconv.ErrSyntax
			}
			id, err := s.store.CreateUserRequestTx(ctx, tx, "bind", req.BillingUsername, nodeID, localUsername, req.Message)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	}); err != nil {
		if err == strconv.ErrSyntax {
			c.JSON(http.StatusBadRequest, gin.H{"error": "node_id/local_username 不能为空"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "request_ids": ids})
}

type openRequestCreateReq struct {
	BillingUsername string `json:"billing_username"`
	NodeID          string `json:"node_id"`
	LocalUsername   string `json:"local_username"`
	Message         string `json:"message"`
}

func validateOpenRequestReason(message string) error {
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) < 20 {
		return errors.New("请详细填写开通理由（至少 20 个字，并说明研究方向）")
	}
	if !strings.Contains(message, "研究方向") &&
		!strings.Contains(message, "研究课题") &&
		!strings.Contains(message, "课题方向") {
		return errors.New("开通理由必须包含“研究方向”描述（例如研究方向/研究课题/课题方向）")
	}
	return nil
}

func (s *Server) handleUserOpenRequestCreate(c *gin.Context) {
	authBilling := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	var req openRequestCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = authBilling
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	req.Message = strings.TrimSpace(req.Message)

	if req.BillingUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing_username 不能为空"})
		return
	}
	if req.NodeID == "" {
		req.NodeID = "待分配"
	}
	if req.LocalUsername == "" {
		req.LocalUsername = "待分配"
	}
	if err := validateOpenRequestReason(req.Message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	var id int
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.store.CreateUserRequestTx(ctx, tx, "open", req.BillingUsername, req.NodeID, req.LocalUsername, req.Message)
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "request_id": id})
}

func (s *Server) handleUserRequestsList(c *gin.Context) {
	billing := strings.TrimSpace(fmt.Sprintf("%v", c.MustGet("auth_user")))
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	records, err := s.store.ListUserRequestsByBilling(c.Request.Context(), billing, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"requests": records})
}

func (s *Server) handleAdminRequestsList(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	records, err := s.store.ListUserRequestsAdmin(c.Request.Context(), status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 附加“同计费账号申请次数”与冲突标记，便于批量审核时快速识别异常。
	countByBilling := map[string]int{}
	countByNodeLocal := map[string]int{}
	for _, r := range records {
		b := strings.TrimSpace(r.BillingUsername)
		if b != "" {
			countByBilling[b]++
		}
		key := strings.TrimSpace(r.NodeID) + "|" + strings.TrimSpace(r.LocalUsername)
		if key != "|" {
			countByNodeLocal[key]++
		}
	}
	type view struct {
		UserRequest
		ApplyCountByBilling int    `json:"apply_count_by_billing"`
		DuplicateFlag       bool   `json:"duplicate_flag"`
		DuplicateReason     string `json:"duplicate_reason,omitempty"`
	}
	out := make([]view, 0, len(records))
	for _, r := range records {
		n := countByBilling[strings.TrimSpace(r.BillingUsername)]
		key := strings.TrimSpace(r.NodeID) + "|" + strings.TrimSpace(r.LocalUsername)
		nNodeLocal := countByNodeLocal[key]
		reasons := make([]string, 0, 2)
		if n > 1 {
			reasons = append(reasons, "计费账号重复申请")
		}
		if nNodeLocal > 1 {
			reasons = append(reasons, "同节点同机器用户名重复申请")
		}
		dup := len(reasons) > 0
		out = append(out, view{
			UserRequest:         r,
			ApplyCountByBilling: n,
			DuplicateFlag:       dup,
			DuplicateReason:     strings.Join(reasons, "；"),
		})
	}
	// 重复申请优先显示，减少人工漏看。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DuplicateFlag != out[j].DuplicateFlag {
			return out[i].DuplicateFlag && !out[j].DuplicateFlag
		}
		return out[i].RequestID > out[j].RequestID
	})
	c.JSON(http.StatusOK, gin.H{"requests": out})
}

func (s *Server) handleAdminRequestApprove(c *gin.Context) {
	s.handleAdminRequestReview(c, "approved")
}

func (s *Server) handleAdminRequestReject(c *gin.Context) {
	s.handleAdminRequestReview(c, "rejected")
}

type adminRequestReviewReq struct {
	Reason string `json:"reason"`
}

func (s *Server) handleAdminRequestReopen(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			reviewedBy = strings.TrimSpace(s)
		}
	} else if v, ok := c.Get("auth_method"); ok {
		if m, ok := v.(string); ok && m == "token" {
			reviewedBy = "admin_token"
		}
	}
	ctx := c.Request.Context()
	now := time.Now()
	var updated UserRequest
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		updated, txErr = s.store.ReopenUserOpenRequestTx(ctx, tx, id, reviewedBy, now)
		return txErr
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "request": updated})
}

func (s *Server) handleAdminRequestReview(c *gin.Context, newStatus string) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			reviewedBy = strings.TrimSpace(s)
		}
	} else if v, ok := c.Get("auth_method"); ok {
		if m, ok := v.(string); ok && m == "token" {
			reviewedBy = "admin_token"
		}
	}
	rejectReason := ""
	if newStatus == "rejected" && c.Request.ContentLength > 0 {
		var req adminRequestReviewReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		rejectReason = strings.TrimSpace(req.Reason)
	}

	ctx := c.Request.Context()
	now := time.Now()
	var updated UserRequest
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		updated, err = s.store.ReviewUserRequestTx(ctx, tx, id, newStatus, reviewedBy, now, rejectReason)
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if newStatus == "approved" && strings.TrimSpace(updated.RequestType) == "unbind" {
		nodeID := strings.TrimSpace(updated.NodeID)
		localUsername := strings.TrimSpace(updated.LocalUsername)
		if nodeID != "" && localUsername != "" {
			s.enqueueNodeAction(nodeID, Action{
				Type:     "kill_all_processes",
				Username: localUsername,
				Reason:   fmt.Sprintf("管理员 %s 审批通过解绑申请：强制终止该账号全部进程", reviewedBy),
			})
			s.enqueueNodeAction(nodeID, Action{
				Type:     "kick_ssh_user",
				Username: localUsername,
				Reason:   fmt.Sprintf("管理员 %s 审批通过解绑申请：强制断开 SSH 会话", reviewedBy),
			})
			s.enqueueNodeAction(nodeID, Action{
				Type:   "force_sync",
				Reason: fmt.Sprintf("管理员 %s 审批通过解绑申请：刷新节点 SSH 策略缓存", reviewedBy),
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "request": updated})
}

type batchReviewReq struct {
	RequestIDs []int  `json:"request_ids"`
	NewStatus  string `json:"new_status"` // approved/rejected
}

type registrationRequestView struct {
	RegistrationRequest
	ConflictFields []string `json:"conflict_fields,omitempty"`
	ConflictReason string   `json:"conflict_reason,omitempty"`
}

type registrationRequestRejectReq struct {
	Reason string `json:"reason"`
}

func isLegalEmailAddress(raw string) bool {
	email := strings.TrimSpace(raw)
	if email == "" {
		return false
	}
	if strings.ContainsAny(email, " \t\r\n") || strings.Count(email, "@") != 1 {
		return false
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return false
	}
	return true
}

func registrationRequestMatchFilter(r RegistrationRequest, field string, keyword string) bool {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return true
	}
	field = strings.TrimSpace(strings.ToLower(field))
	if field == "" {
		field = "all"
	}
	matches := func(v string) bool {
		return strings.Contains(strings.ToLower(strings.TrimSpace(v)), keyword)
	}
	switch field {
	case "username":
		return matches(r.Username)
	case "email":
		return matches(r.Email)
	case "student_id":
		return matches(r.StudentID)
	case "real_name":
		return matches(r.RealName)
	case "advisor":
		return matches(r.Advisor)
	case "phone":
		return matches(r.Phone)
	case "status":
		return matches(r.Status)
	case "all":
		return matches(r.Username) ||
			matches(r.Email) ||
			matches(r.StudentID) ||
			matches(r.RealName) ||
			matches(r.Advisor) ||
			matches(r.Phone) ||
			matches(r.Status)
	default:
		return true
	}
}

func (s *Server) handleAdminRegistrationRequestsOverview(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 1000, 50000)
	field := strings.TrimSpace(c.Query("field"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	pendingRaw, err := s.store.ListRegistrationRequestsAdmin(c.Request.Context(), "pending", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rejectedRaw, err := s.store.ListRegistrationRequestsAdmin(c.Request.Context(), "rejected", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pending := make([]RegistrationRequest, 0, len(pendingRaw))
	for _, row := range pendingRaw {
		if registrationRequestMatchFilter(row, field, keyword) {
			pending = append(pending, row)
		}
	}
	rejected := make([]RegistrationRequest, 0, len(rejectedRaw))
	for _, row := range rejectedRaw {
		if registrationRequestMatchFilter(row, field, keyword) {
			rejected = append(rejected, row)
		}
	}

	pendingNameCount := map[string]int{}
	pendingEmailCount := map[string]int{}
	pendingSIDCount := map[string]int{}
	for _, r := range pending {
		if k := strings.TrimSpace(r.Username); k != "" {
			pendingNameCount[k]++
		}
		if k := strings.TrimSpace(strings.ToLower(r.Email)); k != "" {
			pendingEmailCount[k]++
		}
		if k := strings.TrimSpace(r.StudentID); k != "" {
			pendingSIDCount[k]++
		}
	}

	pendingOut := make([]registrationRequestView, 0, len(pending))
	conflictOut := make([]registrationRequestView, 0)
	for _, r := range pending {
		conflicts := make([]string, 0, 6)
		reasons := make([]string, 0, 6)
		accountConflicts, err := s.store.RegistrationConflictsWithAccounts(c.Request.Context(), r.Username, r.Email, r.StudentID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if accountConflicts["username"] {
			conflicts = append(conflicts, "username")
			reasons = append(reasons, "用户名与已开通平台账号重复")
		}
		if accountConflicts["email"] {
			conflicts = append(conflicts, "email")
			reasons = append(reasons, "邮箱与已开通平台账号重复")
		}
		if accountConflicts["student_id"] {
			conflicts = append(conflicts, "student_id")
			reasons = append(reasons, "学号与已开通平台账号重复")
		}
		if pendingNameCount[strings.TrimSpace(r.Username)] > 1 {
			conflicts = append(conflicts, "username")
			reasons = append(reasons, "用户名与其他待审核申请重复")
		}
		if pendingEmailCount[strings.TrimSpace(strings.ToLower(r.Email))] > 1 {
			conflicts = append(conflicts, "email")
			reasons = append(reasons, "邮箱与其他待审核申请重复")
		}
		if pendingSIDCount[strings.TrimSpace(r.StudentID)] > 1 {
			conflicts = append(conflicts, "student_id")
			reasons = append(reasons, "学号与其他待审核申请重复")
		}

		seen := map[string]struct{}{}
		uniqFields := make([]string, 0, len(conflicts))
		for _, f := range conflicts {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			uniqFields = append(uniqFields, f)
		}
		view := registrationRequestView{
			RegistrationRequest: r,
			ConflictFields:      uniqFields,
			ConflictReason:      strings.Join(reasons, "；"),
		}
		if len(uniqFields) > 0 {
			conflictOut = append(conflictOut, view)
		} else {
			pendingOut = append(pendingOut, view)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"pending":   pendingOut,
		"conflicts": conflictOut,
		"rejected":  rejected,
	})
}

func (s *Server) handleAdminRegistrationRequestApprove(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if x, ok2 := v.(string); ok2 && strings.TrimSpace(x) != "" {
			reviewedBy = strings.TrimSpace(x)
		}
	}
	now := time.Now()
	var updated RegistrationRequest
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var txErr error
		updated, txErr = s.store.ReviewRegistrationRequestTx(c.Request.Context(), tx, id, "approved", reviewedBy, now, "", s.cfg.DefaultBalance)
		return txErr
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 审核通过后通知申请人邮箱（失败不影响审核结果）。
	mailSent := false
	mailErr := ""
	toEmail := strings.TrimSpace(updated.Email)
	if toEmail == "" {
		mailErr = "该申请未填写邮箱，未发送通知"
	} else {
		settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
		if err != nil {
			mailErr = "读取邮件配置失败: " + err.Error()
		} else {
			baseURL := inferRequestBaseURL(c)
			loginURL := "/login"
			if baseURL != "" {
				loginURL = baseURL + "/login"
			}
			subject := "GPU Ops 注册申请已通过"
			body := fmt.Sprintf(
				"你好 %s，\n\n你的平台账号注册申请已通过管理员审核，现在可以登录平台。\n登录地址：%s\n\n建议你登录后尽快在“节点账号”页面填写你已有的计算节点账号映射，以便系统准确识别。\n\nGPU Ops 团队",
				strings.TrimSpace(updated.Username),
				loginURL,
			)
			if err := sendPlainTextMail(settings, toEmail, subject, body); err != nil {
				mailErr = "发送通知邮件失败: " + err.Error()
			} else {
				mailSent = true
			}
		}
	}
	resp := gin.H{
		"ok":        true,
		"request":   updated,
		"mail_sent": mailSent,
	}
	if strings.TrimSpace(mailErr) != "" {
		resp["mail_error"] = mailErr
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleAdminRegistrationRequestReject(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 不合法"})
		return
	}
	var req registrationRequestRejectReq
	_ = c.ShouldBindJSON(&req)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "退回理由不能为空"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if x, ok2 := v.(string); ok2 && strings.TrimSpace(x) != "" {
			reviewedBy = strings.TrimSpace(x)
		}
	}
	now := time.Now()
	var updated RegistrationRequest
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var txErr error
		updated, txErr = s.store.ReviewRegistrationRequestTx(c.Request.Context(), tx, id, "rejected", reviewedBy, now, reason, s.cfg.DefaultBalance)
		return txErr
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 退回后通知申请人邮箱（失败不影响审核结果）。
	mailSent := false
	mailErr := ""
	toEmail := strings.TrimSpace(updated.Email)
	if toEmail == "" {
		mailErr = "该申请未填写邮箱，未发送通知"
	} else if !isLegalEmailAddress(toEmail) {
		mailErr = "邮箱格式不合法，已跳过发送通知邮件"
	} else {
		settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
		if err != nil {
			mailErr = "读取邮件配置失败: " + err.Error()
		} else {
			reasonText := strings.TrimSpace(updated.RejectReason)
			if reasonText == "" {
				reasonText = "（未填写）"
			}
			subject := "GPU Ops 注册申请退回通知"
			body := fmt.Sprintf(
				"你好 %s，\n\n你的平台账号注册申请已被管理员退回。\n退回理由：%s\n\n请修改后重新提交注册申请。\n\nGPU Ops 团队",
				strings.TrimSpace(updated.Username),
				reasonText,
			)
			if err := sendPlainTextMail(settings, toEmail, subject, body); err != nil {
				mailErr = "发送通知邮件失败: " + err.Error()
			} else {
				mailSent = true
			}
		}
	}
	mailRecorded := true
	if err := s.store.SetRegistrationRejectNotifyMailResult(c.Request.Context(), updated.RequestID, mailSent, mailErr); err != nil {
		mailRecorded = false
		if strings.TrimSpace(mailErr) == "" {
			mailErr = "记录邮件通知状态失败: " + err.Error()
		} else {
			mailErr = strings.TrimSpace(mailErr) + "；记录邮件通知状态失败: " + err.Error()
		}
	}
	updated.RejectNotifyMailChecked = mailRecorded
	updated.RejectNotifyMailSent = mailSent
	updated.RejectNotifyMailError = strings.TrimSpace(mailErr)

	resp := gin.H{
		"ok":        true,
		"request":   updated,
		"mail_sent": mailSent,
	}
	if strings.TrimSpace(mailErr) != "" {
		resp["mail_error"] = mailErr
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleAdminRequestsBatchReview(c *gin.Context) {
	var req batchReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.NewStatus = strings.TrimSpace(req.NewStatus)
	if req.NewStatus != "approved" && req.NewStatus != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_status 仅支持 approved/rejected"})
		return
	}
	if len(req.RequestIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_ids 不能为空"})
		return
	}
	reviewedBy := "admin"
	if v, ok := c.Get("auth_user"); ok {
		if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
			reviewedBy = strings.TrimSpace(s)
		}
	}
	now := time.Now()
	okCount := 0
	failCount := 0
	failItems := make([]gin.H, 0)
	defaultRejectReason := "管理员批量拒绝"
	for _, id := range req.RequestIDs {
		if id <= 0 {
			failCount++
			failItems = append(failItems, gin.H{"request_id": id, "error": "id 不合法"})
			continue
		}
		err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
			rejectReason := ""
			if req.NewStatus == "rejected" {
				rejectReason = defaultRejectReason
			}
			updated, err := s.store.ReviewUserRequestTx(c.Request.Context(), tx, id, req.NewStatus, reviewedBy, now, rejectReason)
			if err != nil {
				return err
			}
			if req.NewStatus == "approved" && strings.TrimSpace(updated.RequestType) == "unbind" {
				nodeID := strings.TrimSpace(updated.NodeID)
				localUsername := strings.TrimSpace(updated.LocalUsername)
				if nodeID != "" && localUsername != "" {
					s.enqueueNodeAction(nodeID, Action{
						Type:     "kill_all_processes",
						Username: localUsername,
						Reason:   fmt.Sprintf("管理员 %s 批量审批解绑通过：强制终止该账号全部进程", reviewedBy),
					})
					s.enqueueNodeAction(nodeID, Action{
						Type:     "kick_ssh_user",
						Username: localUsername,
						Reason:   fmt.Sprintf("管理员 %s 批量审批解绑通过：强制断开 SSH 会话", reviewedBy),
					})
					s.enqueueNodeAction(nodeID, Action{
						Type:   "force_sync",
						Reason: fmt.Sprintf("管理员 %s 批量审批解绑通过：刷新节点 SSH 策略缓存", reviewedBy),
					})
				}
			}
			return err
		})
		if err != nil {
			failCount++
			failItems = append(failItems, gin.H{"request_id": id, "error": err.Error()})
			continue
		}
		okCount++
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"ok_count":   okCount,
		"fail_count": failCount,
		"fail_items": failItems,
	})
}
