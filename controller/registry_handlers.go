package main

import (
	"database/sql"
	"fmt"
	"net/http"
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

	exclusiveEnabled, err := s.store.GetNodeSSHExclusivePolicy(ctx, nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exclusiveEnabled {
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
	exclusiveEnabled, err := s.store.GetNodeSSHExclusivePolicy(c.Request.Context(), nodeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"node_id":           nodeID,
		"guard_enabled":     guardEnabled,
		"exclusive_enabled": exclusiveEnabled,
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
	var req bindRequestsCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
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

func (s *Server) handleUserOpenRequestCreate(c *gin.Context) {
	var req openRequestCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.BillingUsername = strings.TrimSpace(req.BillingUsername)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.LocalUsername = strings.TrimSpace(req.LocalUsername)
	req.Message = strings.TrimSpace(req.Message)

	if req.BillingUsername == "" || req.NodeID == "" || req.LocalUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing_username/node_id/local_username 不能为空"})
		return
	}
	if utf8.RuneCountInString(req.Message) < 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请详细填写开通理由（至少 10 个字）"})
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
	billing := strings.TrimSpace(c.Query("billing_username"))
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

	ctx := c.Request.Context()
	now := time.Now()
	var updated UserRequest
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		updated, err = s.store.ReviewUserRequestTx(ctx, tx, id, newStatus, reviewedBy, now)
		return err
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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

func (s *Server) handleAdminRegistrationRequestsOverview(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 500, 5000)
	pending, err := s.store.ListRegistrationRequestsAdmin(c.Request.Context(), "pending", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rejected, err := s.store.ListRegistrationRequestsAdmin(c.Request.Context(), "rejected", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
	for _, id := range req.RequestIDs {
		if id <= 0 {
			failCount++
			failItems = append(failItems, gin.H{"request_id": id, "error": "id 不合法"})
			continue
		}
		err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
			_, err := s.store.ReviewUserRequestTx(c.Request.Context(), tx, id, req.NewStatus, reviewedBy, now)
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
