package main

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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
