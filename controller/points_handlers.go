package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminPointsAdjustReq struct {
	Username string  `json:"username"`
	Delta    float64 `json:"delta"`
	Reason   string  `json:"reason"`
	Scope    string  `json:"scope"`   // general | carryover | node_exclusive
	NodeID   string  `json:"node_id"` // scope=node_exclusive 时必填
}

type adminPointsBatchGrantReq struct {
	Amount float64 `json:"amount"`
	Reason string  `json:"reason"`
	Scope  string  `json:"scope"`   // general | carryover | node_exclusive
	NodeID string  `json:"node_id"` // scope=node_exclusive 时必填
}

type adminPointsBatchAdjustUsersReq struct {
	Amount    float64  `json:"amount"`
	Reason    string   `json:"reason"`
	Usernames []string `json:"usernames"`
	Scope     string   `json:"scope"`   // general | carryover | node_exclusive
	NodeID    string   `json:"node_id"` // scope=node_exclusive 时必填
}

type adminPointsBatchSetUsersReq struct {
	Value     float64  `json:"value"`
	Reason    string   `json:"reason"`
	Usernames []string `json:"usernames"`
	Scope     string   `json:"scope"`   // general | carryover | node_exclusive
	NodeID    string   `json:"node_id"` // scope=node_exclusive 时必填
}

type adminPointsSpecialRuleReq struct {
	Username      string  `json:"username"`
	MonthlyPoints float64 `json:"monthly_points"`
	Enabled       bool    `json:"enabled"`
	Note          string  `json:"note"`
}

type adminPointsMonthlyResetReq struct {
	Force bool `json:"force"`
}

type adminPointsMonthlyConfigReq struct {
	DoctorPoints      float64 `json:"doctor_points"`
	MasterPoints      float64 `json:"master_points"`
	OtherPoints       float64 `json:"other_points"`
	CarryoverLimit    float64 `json:"carryover_limit"`
	MaxOverdraftLimit float64 `json:"max_overdraft_limit"`
}

type pointsMonthlyResetSummary struct {
	MonthKey          string                 `json:"month_key"`
	AlreadyRun        bool                   `json:"already_run"`
	Run               *MonthlyPointsResetRun `json:"run,omitempty"`
	TotalUsers        int                    `json:"total_users"`
	ChangedUsers      int                    `json:"changed_users"`
	InterruptedUsers  int                    `json:"interrupted_users"`
	InterruptedTarget int                    `json:"interrupted_targets"`
	QuotaRefreshUsers int                    `json:"quota_refresh_users"`
	QuotaRefreshNodes int                    `json:"quota_refresh_nodes"`
	QuotaRefreshErrs  int                    `json:"quota_refresh_errors"`
}

type billingUserTarget struct {
	NodeID        string
	LocalUsername string
}

func monthlyBasePointsByStudentID(studentID string, cfg MonthlyPointsConfig) float64 {
	sid := strings.ToUpper(strings.TrimSpace(studentID))
	if strings.Contains(sid, "B") {
		return cfg.DoctorPoints
	}
	if strings.Contains(sid, "S") {
		return cfg.MasterPoints
	}
	return cfg.OtherPoints
}

func normalizePointsAdjustScope(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "general"
	}
	if scope != "general" && scope != "carryover" && scope != "node_exclusive" {
		return "", errors.New("scope 仅支持 general、carryover 或 node_exclusive")
	}
	return scope, nil
}

func killAllProcessBalanceThreshold(maxOverdraftLimit float64) float64 {
	limit := normalizeMonthlyMaxOverdraftLimit(maxOverdraftLimit)
	return -limit
}

func isOverdraftExceeded(balance float64, maxOverdraftLimit float64) bool {
	return balance < killAllProcessBalanceThreshold(maxOverdraftLimit)
}

func shouldTriggerOverdraftInterrupt(prevBalance float64, nextBalance float64, maxOverdraftLimit float64) bool {
	return !isOverdraftExceeded(prevBalance, maxOverdraftLimit) && isOverdraftExceeded(nextBalance, maxOverdraftLimit)
}

func (s *Server) getMonthlyMaxOverdraftLimit(ctx context.Context) (float64, error) {
	cfg, err := s.store.GetMonthlyPointsConfig(ctx)
	if err != nil {
		return 0, err
	}
	return normalizeMonthlyMaxOverdraftLimit(cfg.MaxOverdraftLimit), nil
}

func (s *Server) listBillingUserTargets(ctx context.Context, billingUsername string) ([]billingUserTarget, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return nil, errors.New("billing_username 不能为空")
	}
	targets := map[string]billingUserTarget{}
	add := func(nodeID string, localUsername string) {
		nodeID = strings.TrimSpace(nodeID)
		localUsername = strings.TrimSpace(localUsername)
		if nodeID == "" || localUsername == "" {
			return
		}
		targets[nodeID+"|"+localUsername] = billingUserTarget{
			NodeID:        nodeID,
			LocalUsername: localUsername,
		}
	}

	accounts, err := s.store.ListUserNodeAccountsByBilling(ctx, billingUsername, 5000)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	for _, acc := range accounts {
		add(acc.NodeID, acc.LocalUsername)
	}

	// 没有映射时，按“平台用户名 == 节点本地用户名”兜底处理所有节点。
	if len(targets) == 0 {
		nodes, err := s.store.ListNodes(ctx, 5000)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			add(n.NodeID, billingUsername)
		}
	}

	if len(targets) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]billingUserTarget, 0, len(keys))
	for _, k := range keys {
		out = append(out, targets[k])
	}
	return out, nil
}

func (s *Server) enqueueHardInterruptForBillingUser(ctx context.Context, billingUsername string, maxOverdraftLimit float64, reason string) (int, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = fmt.Sprintf("欠费超过 %.2f 积分上限，强制中断所有进程", normalizeMonthlyMaxOverdraftLimit(maxOverdraftLimit))
	}
	targets, err := s.listBillingUserTargets(ctx, billingUsername)
	if err != nil {
		return 0, err
	}
	for _, t := range targets {
		nodeID := t.NodeID
		localUsername := t.LocalUsername
		s.enqueueNodeAction(nodeID, Action{
			Type:     "kill_all_processes",
			Username: localUsername,
			Reason:   reason,
		})
	}
	return len(targets), nil
}

func (s *Server) applyMonthlyPointsReset(ctx context.Context, runBy string, force bool) (pointsMonthlyResetSummary, error) {
	s.pointsResetMu.Lock()
	defer s.pointsResetMu.Unlock()

	now := time.Now()
	monthKey := now.In(time.Local).Format("2006-01")
	out := pointsMonthlyResetSummary{MonthKey: monthKey}

	if !force {
		run, exists, err := s.store.GetMonthlyPointsResetRun(ctx, monthKey)
		if err != nil {
			return out, err
		}
		if exists {
			out.AlreadyRun = true
			out.Run = &run
			return out, nil
		}
	}

	users, err := s.store.ListRegularUserProfilesForPoints(ctx, 50000)
	if err != nil {
		return out, err
	}
	out.TotalUsers = len(users)

	interruptUsers := map[string]struct{}{}
	stateRecheckUsers := map[string]float64{}
	changedUsers := 0
	monthlyCfg := MonthlyPointsConfig{
		DoctorPoints:      200,
		MasterPoints:      100,
		OtherPoints:       50,
		CarryoverLimit:    500,
		MaxOverdraftLimit: 500,
	}
	method := "monthly_reset"
	carryMethod := "monthly_carryover_reset"
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if !force {
			exists, err := s.store.IsMonthlyPointsResetRunExistsTx(ctx, tx, monthKey)
			if err != nil {
				return err
			}
			if exists {
				out.AlreadyRun = true
				return nil
			}
		}

		loadedCfg, err := s.store.LoadMonthlyPointsConfigTx(ctx, tx)
		if err != nil {
			return err
		}
		monthlyCfg = loadedCfg
		specialMap, err := s.store.LoadSpecialMonthlyPointsMapTx(ctx, tx)
		if err != nil {
			return err
		}

		for _, u := range users {
			target := monthlyBasePointsByStudentID(u.StudentID, monthlyCfg)
			if v, ok := specialMap[u.Username]; ok {
				target = v
			}
			prevUsable := u.GeneralBalance + u.CarryoverBalance
			targetCarry := prevUsable
			if targetCarry < 0 {
				targetCarry = 0
			}
			if targetCarry > monthlyCfg.CarryoverLimit {
				targetCarry = monthlyCfg.CarryoverLimit
			}
			res, deltaGeneral, deltaCarryover, err := s.store.SetGeneralAndCarryoverBalanceTx(
				ctx,
				tx,
				u.Username,
				target,
				targetCarry,
				method,
				carryMethod,
				now,
				s.cfg,
			)
			if err != nil {
				return err
			}
			if deltaGeneral != 0 || deltaCarryover != 0 {
				changedUsers++
				stateRecheckUsers[u.Username] = res.User.Balance + res.User.CarryoverBalance
			}
			if shouldTriggerOverdraftInterrupt(res.PrevEffectiveBalance, res.EffectiveBalance, monthlyCfg.MaxOverdraftLimit) {
				interruptUsers[u.Username] = struct{}{}
			}
		}

		return s.store.UpsertMonthlyPointsResetRunTx(ctx, tx, monthKey, runBy, len(users), changedUsers, force)
	}); err != nil {
		return out, err
	}
	if out.AlreadyRun {
		run, exists, err := s.store.GetMonthlyPointsResetRun(ctx, monthKey)
		if err == nil && exists {
			out.Run = &run
		}
		return out, nil
	}

	out.ChangedUsers = changedUsers
	interruptedTargets := 0
	killThreshold := killAllProcessBalanceThreshold(monthlyCfg.MaxOverdraftLimit)
	for username := range interruptUsers {
		n, err := s.enqueueHardInterruptForBillingUser(
			ctx,
			username,
			monthlyCfg.MaxOverdraftLimit,
			fmt.Sprintf("月初重置后余额低于 %.2f 积分，已超过每月最大欠费上限，强制中断", killThreshold),
		)
		if err != nil {
			log.Printf("月初重置后强制中断失败 username=%s err=%v", username, err)
			continue
		}
		if n > 0 {
			out.InterruptedUsers++
			interruptedTargets += n
		}
	}
	out.InterruptedTarget = interruptedTargets
	if len(stateRecheckUsers) > 0 {
		usernames := make([]string, 0, len(stateRecheckUsers))
		for username := range stateRecheckUsers {
			usernames = append(usernames, username)
		}
		sort.Strings(usernames)
		for _, username := range usernames {
			n, err := s.refreshCPUQuotaForBillingUser(
				ctx,
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("月初重置后积分调整，重算限速（%s）", username),
			)
			if err != nil {
				out.QuotaRefreshErrs++
				log.Printf("月初重置后刷新 CPU 限速失败 username=%s err=%v", username, err)
			} else if n > 0 {
				out.QuotaRefreshUsers++
				out.QuotaRefreshNodes += n
			}
			if _, err := s.refreshGPUAccessForBillingUser(
				ctx,
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("月初重置后积分调整，同步 GPU 限制（%s）", username),
			); err != nil {
				log.Printf("月初重置后刷新 GPU 限制失败 username=%s err=%v", username, err)
			}
			if _, err := s.refreshMemoryLimitForBillingUser(
				ctx,
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("月初重置后积分调整，同步内存限额（%s）", username),
			); err != nil {
				log.Printf("月初重置后刷新内存限额失败 username=%s err=%v", username, err)
			}
		}
	}

	run, exists, err := s.store.GetMonthlyPointsResetRun(ctx, monthKey)
	if err == nil && exists {
		out.Run = &run
	}
	return out, nil
}

func (s *Server) StartPointsMonthlyResetScheduler(ctx context.Context) {
	tryAutoReset := func() {
		now := time.Now().In(time.Local)
		if now.Day() != 1 {
			return
		}
		if _, err := s.applyMonthlyPointsReset(context.Background(), "system", false); err != nil {
			log.Printf("月初积分重置自动任务异常：%v", err)
		}
	}
	go func() {
		// 启动后立即检查一次（仅在每月 1 号自动执行）。
		tryAutoReset()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tryAutoReset()
			}
		}
	}()
}

func (s *Server) handleAdminPointsUsers(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	keywordField := normalizePointsUsersKeywordField(c.Query("keyword_field"))
	limit := parseLimit(c.Query("limit"), 1000, 5000)
	rows, err := s.store.ListPointsUsers(c.Request.Context(), keyword, keywordField, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"users": rows})
}

func normalizePointsUsersKeywordField(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "username", "student_id", "real_name", "advisor", "email", "phone":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "all"
	}
}

func (s *Server) handleAdminPointsRecords(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	method := strings.TrimSpace(c.Query("method"))
	limit := parseLimit(c.Query("limit"), 500, 5000)
	rows, err := s.store.ListPointsOperationRecords(c.Request.Context(), username, method, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"records": rows})
}

func (s *Server) handleAdminPointsAdjust(c *gin.Context) {
	var req adminPointsAdjustReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(400, gin.H{"error": "username 不能为空"})
		return
	}
	if req.Delta == 0 {
		c.JSON(400, gin.H{"error": "delta 不能为 0"})
		return
	}
	var scopeErr error
	req.Scope, scopeErr = normalizePointsAdjustScope(req.Scope)
	req.NodeID = strings.TrimSpace(req.NodeID)
	if scopeErr != nil {
		c.JSON(400, gin.H{"error": scopeErr.Error()})
		return
	}
	if req.Scope == "node_exclusive" && req.NodeID == "" {
		c.JSON(400, gin.H{"error": "node_exclusive 模式下 node_id 不能为空"})
		return
	}
	method := "points_adjust_plus"
	if req.Delta < 0 {
		method = "points_adjust_minus"
	}
	if req.Scope == "carryover" {
		if req.Delta < 0 {
			method = "points_adjust_carry_minus"
		} else {
			method = "points_adjust_carry_plus"
		}
	}
	if req.Scope == "node_exclusive" {
		if req.Delta < 0 {
			method = "points_adjust_node_minus"
		} else {
			method = "points_adjust_node_plus"
		}
	}
	now := time.Now()
	actionCtx := withRechargeReason(c.Request.Context(), req.Reason)
	var res BalanceUpdateResult
	carryoverBalance := 0.0
	exclusiveBalance := 0.0
	nodeExclusiveCurrent := 0.0
	if err := s.store.WithTx(actionCtx, func(tx *sql.Tx) error {
		var err error
		if req.Scope == "node_exclusive" {
			row, rowErr := s.store.AdjustNodeExclusiveBalanceTx(
				actionCtx,
				tx,
				req.Username,
				req.NodeID,
				req.Delta,
				method,
				s.currentOperator(c),
				s.cfg,
			)
			if rowErr != nil {
				return rowErr
			}
			nodeExclusiveCurrent = row.Balance
			u, getErr := s.store.EnsureUserTx(actionCtx, tx, req.Username, s.cfg.DefaultBalance)
			if getErr != nil {
				return getErr
			}
			exclusiveBalance, err = s.store.GetUserExclusiveBalanceTotalTx(actionCtx, tx, req.Username)
			if err != nil {
				return err
			}
			carryoverBalance = u.CarryoverBalance
			res = BalanceUpdateResult{
				PrevStatus: u.Status,
				User:       u,
			}
			return nil
		}
		if req.Scope == "carryover" {
			res, err = s.store.AdjustCarryoverBalanceTx(actionCtx, tx, req.Username, req.Delta, method, now, s.cfg)
		} else {
			res, err = s.store.AdjustBalanceTx(actionCtx, tx, req.Username, req.Delta, method, now, s.cfg)
		}
		if err != nil {
			return err
		}
		carryoverBalance = res.User.CarryoverBalance
		exclusiveBalance, err = s.store.GetUserExclusiveBalanceTotalTx(actionCtx, tx, req.Username)
		return err
	}); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	interruptTargets := 0
	maxOverdraftLimit := 0.0
	if req.Scope == "general" || req.Scope == "carryover" {
		limit, err := s.getMonthlyMaxOverdraftLimit(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": "积分已调整，但读取欠费上限失败: " + err.Error()})
			return
		}
		maxOverdraftLimit = limit
	}
	if (req.Scope == "general" || req.Scope == "carryover") && shouldTriggerOverdraftInterrupt(res.PrevEffectiveBalance, res.EffectiveBalance, maxOverdraftLimit) {
		killThreshold := killAllProcessBalanceThreshold(maxOverdraftLimit)
		n, err := s.enqueueHardInterruptForBillingUser(
			c.Request.Context(),
			req.Username,
			maxOverdraftLimit,
			fmt.Sprintf("管理员调整后余额低于 %.2f 积分，已超过每月最大欠费上限，强制中断", killThreshold),
		)
		if err != nil {
			c.JSON(500, gin.H{"error": "积分已调整，但下发强制中断失败: " + err.Error()})
			return
		}
		interruptTargets = n
	}
	quotaRefreshTargets := 0
	quotaRefreshError := ""
	gpuRefreshTargets := 0
	gpuRefreshError := ""
	memoryRefreshTargets := 0
	memoryRefreshError := ""
	if req.Scope == "general" || req.Scope == "carryover" {
		n, err := s.refreshCPUQuotaForBillingUser(
			c.Request.Context(),
			req.Username,
			res.User.Balance+carryoverBalance,
			fmt.Sprintf("管理员积分调整后重算限速（%s）", req.Username),
		)
		if err != nil {
			quotaRefreshError = err.Error()
			log.Printf("积分调整后刷新 CPU 限速失败 username=%s err=%v", req.Username, err)
		} else {
			quotaRefreshTargets = n
		}
		n, err = s.refreshGPUAccessForBillingUser(
			c.Request.Context(),
			req.Username,
			res.User.Balance+carryoverBalance,
			fmt.Sprintf("管理员积分调整后同步 GPU 限制（%s）", req.Username),
		)
		if err != nil {
			gpuRefreshError = err.Error()
			log.Printf("积分调整后刷新 GPU 限制失败 username=%s err=%v", req.Username, err)
		} else {
			gpuRefreshTargets = n
		}
		n, err = s.refreshMemoryLimitForBillingUser(
			c.Request.Context(),
			req.Username,
			res.User.Balance+carryoverBalance,
			fmt.Sprintf("管理员积分调整后同步内存限额（%s）", req.Username),
		)
		if err != nil {
			memoryRefreshError = err.Error()
			log.Printf("积分调整后刷新内存限额失败 username=%s err=%v", req.Username, err)
		} else {
			memoryRefreshTargets = n
		}
	}
	totalBalance := res.User.Balance + carryoverBalance + exclusiveBalance

	c.JSON(200, gin.H{
		"ok":                             true,
		"scope":                          req.Scope,
		"node_id":                        req.NodeID,
		"username":                       res.User.Username,
		"balance":                        res.User.Balance,
		"general_balance":                res.User.Balance,
		"carryover_balance":              carryoverBalance,
		"exclusive_balance":              exclusiveBalance,
		"total_balance":                  totalBalance,
		"node_exclusive_balance_current": nodeExclusiveCurrent,
		"status":                         res.User.Status,
		"interrupt_targets":              interruptTargets,
		"quota_refresh_targets":          quotaRefreshTargets,
		"quota_refresh_error":            quotaRefreshError,
		"gpu_refresh_targets":            gpuRefreshTargets,
		"gpu_refresh_error":              gpuRefreshError,
		"memory_refresh_targets":         memoryRefreshTargets,
		"memory_refresh_error":           memoryRefreshError,
	})
}

func (s *Server) handleAdminPointsBatchGrant(c *gin.Context) {
	var req adminPointsBatchGrantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	if req.Amount == 0 {
		c.JSON(400, gin.H{"error": "amount 不能为 0"})
		return
	}
	var scopeErr error
	req.Scope, scopeErr = normalizePointsAdjustScope(req.Scope)
	req.NodeID = strings.TrimSpace(req.NodeID)
	if scopeErr != nil {
		c.JSON(400, gin.H{"error": scopeErr.Error()})
		return
	}
	if req.Scope == "node_exclusive" && req.NodeID == "" {
		c.JSON(400, gin.H{"error": "node_exclusive 模式下 node_id 不能为空"})
		return
	}
	users, err := s.store.ListRegularUserProfilesForPoints(c.Request.Context(), 50000)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	maxOverdraftLimit := 0.0
	if req.Scope == "general" || req.Scope == "carryover" {
		maxOverdraftLimit, err = s.getMonthlyMaxOverdraftLimit(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	killThreshold := killAllProcessBalanceThreshold(maxOverdraftLimit)
	now := time.Now()
	changed := 0
	interruptUsers := map[string]struct{}{}
	stateRecheckUsers := map[string]float64{}
	method := "points_batch_plus"
	if req.Scope == "carryover" {
		method = "points_batch_carry_plus"
		if req.Amount < 0 {
			method = "points_batch_carry_minus"
		}
	} else if req.Scope == "node_exclusive" {
		method = "points_batch_node_plus"
		if req.Amount < 0 {
			method = "points_batch_node_minus"
		}
	} else if req.Amount < 0 {
		method = "points_batch_minus"
	}
	operator := s.currentOperator(c)
	actionCtx := withRechargeReason(c.Request.Context(), req.Reason)
	if err := s.store.WithTx(actionCtx, func(tx *sql.Tx) error {
		for _, u := range users {
			if req.Scope == "node_exclusive" {
				if _, err := s.store.AdjustNodeExclusiveBalanceTx(
					actionCtx,
					tx,
					u.Username,
					req.NodeID,
					req.Amount,
					method,
					operator,
					s.cfg,
				); err != nil {
					return err
				}
				changed++
				continue
			}
			var res BalanceUpdateResult
			var err error
			if req.Scope == "carryover" {
				res, err = s.store.AdjustCarryoverBalanceTx(actionCtx, tx, u.Username, req.Amount, method, now, s.cfg)
			} else {
				res, err = s.store.AdjustBalanceTx(actionCtx, tx, u.Username, req.Amount, method, now, s.cfg)
			}
			if err != nil {
				return err
			}
			changed++
			stateRecheckUsers[u.Username] = res.User.Balance + res.User.CarryoverBalance
			if shouldTriggerOverdraftInterrupt(res.PrevEffectiveBalance, res.EffectiveBalance, maxOverdraftLimit) {
				interruptUsers[u.Username] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	interruptedUsers := 0
	interruptedNodes := 0
	if req.Scope == "general" || req.Scope == "carryover" {
		for username := range interruptUsers {
			n, err := s.enqueueHardInterruptForBillingUser(
				c.Request.Context(),
				username,
				maxOverdraftLimit,
				fmt.Sprintf("全体积分调整后余额低于 %.2f 积分，已超过每月最大欠费上限，强制中断", killThreshold),
			)
			if err != nil {
				log.Printf("全体积分调整后强制中断失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				interruptedUsers++
				interruptedNodes += n
			}
		}
	}
	quotaRefreshUsers := 0
	quotaRefreshNodes := 0
	quotaRefreshErrs := 0
	gpuRefreshUsers := 0
	gpuRefreshNodes := 0
	gpuRefreshErrs := 0
	memoryRefreshUsers := 0
	memoryRefreshNodes := 0
	memoryRefreshErrs := 0
	if (req.Scope == "general" || req.Scope == "carryover") && len(stateRecheckUsers) > 0 {
		usernames := make([]string, 0, len(stateRecheckUsers))
		for username := range stateRecheckUsers {
			usernames = append(usernames, username)
		}
		sort.Strings(usernames)
		for _, username := range usernames {
			n, err := s.refreshCPUQuotaForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("全体积分调整后重算限速（%s）", username),
			)
			if err != nil {
				quotaRefreshErrs++
				log.Printf("全体积分调整后刷新 CPU 限速失败 username=%s err=%v", username, err)
			} else if n > 0 {
				quotaRefreshUsers++
				quotaRefreshNodes += n
			}
			n, err = s.refreshGPUAccessForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("全体积分调整后同步 GPU 限制（%s）", username),
			)
			if err != nil {
				gpuRefreshErrs++
				log.Printf("全体积分调整后刷新 GPU 限制失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				gpuRefreshUsers++
				gpuRefreshNodes += n
			}
			n, err = s.refreshMemoryLimitForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("全体积分调整后同步内存限额（%s）", username),
			)
			if err != nil {
				memoryRefreshErrs++
				log.Printf("全体积分调整后刷新内存限额失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				memoryRefreshUsers++
				memoryRefreshNodes += n
			}
		}
	}

	c.JSON(200, gin.H{
		"ok":                    true,
		"scope":                 req.Scope,
		"node_id":               req.NodeID,
		"amount":                req.Amount,
		"granted_users":         changed,
		"adjusted_users":        changed,
		"total_granted":         req.Amount * float64(changed),
		"total_adjusted":        req.Amount * float64(changed),
		"interrupted_users":     interruptedUsers,
		"interrupted_nodes":     interruptedNodes,
		"quota_refresh_users":   quotaRefreshUsers,
		"quota_refresh_nodes":   quotaRefreshNodes,
		"quota_refresh_errors":  quotaRefreshErrs,
		"gpu_refresh_users":     gpuRefreshUsers,
		"gpu_refresh_nodes":     gpuRefreshNodes,
		"gpu_refresh_errors":    gpuRefreshErrs,
		"memory_refresh_users":  memoryRefreshUsers,
		"memory_refresh_nodes":  memoryRefreshNodes,
		"memory_refresh_errors": memoryRefreshErrs,
	})
}

func (s *Server) handleAdminPointsBatchAdjustUsers(c *gin.Context) {
	var req adminPointsBatchAdjustUsersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	if req.Amount == 0 {
		c.JSON(400, gin.H{"error": "amount 不能为 0"})
		return
	}
	var scopeErr error
	req.Scope, scopeErr = normalizePointsAdjustScope(req.Scope)
	req.NodeID = strings.TrimSpace(req.NodeID)
	if scopeErr != nil {
		c.JSON(400, gin.H{"error": scopeErr.Error()})
		return
	}
	if req.Scope == "node_exclusive" && req.NodeID == "" {
		c.JSON(400, gin.H{"error": "node_exclusive 模式下 node_id 不能为空"})
		return
	}
	targetUsernames := uniqTrim(req.Usernames)
	if len(targetUsernames) == 0 {
		c.JSON(400, gin.H{"error": "usernames 不能为空"})
		return
	}
	if len(targetUsernames) > 50000 {
		c.JSON(400, gin.H{"error": "批量用户数量过大（最大 50000）"})
		return
	}
	existingUsernames, err := s.store.FilterExistingPointsUsernames(c.Request.Context(), targetUsernames)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if len(existingUsernames) == 0 {
		c.JSON(400, gin.H{"error": "筛选结果为空或用户不存在"})
		return
	}
	maxOverdraftLimit := 0.0
	if req.Scope == "general" || req.Scope == "carryover" {
		maxOverdraftLimit, err = s.getMonthlyMaxOverdraftLimit(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	killThreshold := killAllProcessBalanceThreshold(maxOverdraftLimit)

	now := time.Now()
	changed := 0
	interruptUsers := map[string]struct{}{}
	stateRecheckUsers := map[string]float64{}
	method := "points_batch_filtered_plus"
	if req.Scope == "carryover" {
		method = "points_batch_filtered_carry_plus"
		if req.Amount < 0 {
			method = "points_batch_filtered_carry_minus"
		}
	} else if req.Scope == "node_exclusive" {
		method = "points_batch_filtered_node_plus"
		if req.Amount < 0 {
			method = "points_batch_filtered_node_minus"
		}
	} else if req.Amount < 0 {
		method = "points_batch_filtered_minus"
	}
	operator := s.currentOperator(c)
	actionCtx := withRechargeReason(c.Request.Context(), req.Reason)
	if err := s.store.WithTx(actionCtx, func(tx *sql.Tx) error {
		for _, username := range existingUsernames {
			if req.Scope == "node_exclusive" {
				if _, err := s.store.AdjustNodeExclusiveBalanceTx(
					actionCtx,
					tx,
					username,
					req.NodeID,
					req.Amount,
					method,
					operator,
					s.cfg,
				); err != nil {
					return err
				}
				changed++
				continue
			}
			var res BalanceUpdateResult
			var err error
			if req.Scope == "carryover" {
				res, err = s.store.AdjustCarryoverBalanceTx(actionCtx, tx, username, req.Amount, method, now, s.cfg)
			} else {
				res, err = s.store.AdjustBalanceTx(actionCtx, tx, username, req.Amount, method, now, s.cfg)
			}
			if err != nil {
				return err
			}
			changed++
			stateRecheckUsers[username] = res.User.Balance + res.User.CarryoverBalance
			if shouldTriggerOverdraftInterrupt(res.PrevEffectiveBalance, res.EffectiveBalance, maxOverdraftLimit) {
				interruptUsers[username] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	interruptedUsers := 0
	interruptedNodes := 0
	if req.Scope == "general" || req.Scope == "carryover" {
		for username := range interruptUsers {
			n, err := s.enqueueHardInterruptForBillingUser(
				c.Request.Context(),
				username,
				maxOverdraftLimit,
				fmt.Sprintf("批量积分调整后余额低于 %.2f 积分，已超过每月最大欠费上限，强制中断", killThreshold),
			)
			if err != nil {
				log.Printf("筛选批量积分调整后强制中断失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				interruptedUsers++
				interruptedNodes += n
			}
		}
	}
	quotaRefreshUsers := 0
	quotaRefreshNodes := 0
	quotaRefreshErrs := 0
	gpuRefreshUsers := 0
	gpuRefreshNodes := 0
	gpuRefreshErrs := 0
	memoryRefreshUsers := 0
	memoryRefreshNodes := 0
	memoryRefreshErrs := 0
	if (req.Scope == "general" || req.Scope == "carryover") && len(stateRecheckUsers) > 0 {
		usernames := make([]string, 0, len(stateRecheckUsers))
		for username := range stateRecheckUsers {
			usernames = append(usernames, username)
		}
		sort.Strings(usernames)
		for _, username := range usernames {
			n, err := s.refreshCPUQuotaForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分调整后重算限速（%s）", username),
			)
			if err != nil {
				quotaRefreshErrs++
				log.Printf("筛选批量积分调整后刷新 CPU 限速失败 username=%s err=%v", username, err)
			} else if n > 0 {
				quotaRefreshUsers++
				quotaRefreshNodes += n
			}
			n, err = s.refreshGPUAccessForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分调整后同步 GPU 限制（%s）", username),
			)
			if err != nil {
				gpuRefreshErrs++
				log.Printf("筛选批量积分调整后刷新 GPU 限制失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				gpuRefreshUsers++
				gpuRefreshNodes += n
			}
			n, err = s.refreshMemoryLimitForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分调整后同步内存限额（%s）", username),
			)
			if err != nil {
				memoryRefreshErrs++
				log.Printf("筛选批量积分调整后刷新内存限额失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				memoryRefreshUsers++
				memoryRefreshNodes += n
			}
		}
	}

	c.JSON(200, gin.H{
		"ok":                    true,
		"scope":                 req.Scope,
		"node_id":               req.NodeID,
		"amount":                req.Amount,
		"requested_users":       len(targetUsernames),
		"matched_users":         len(existingUsernames),
		"adjusted_users":        changed,
		"skipped_users":         len(targetUsernames) - len(existingUsernames),
		"total_adjusted":        req.Amount * float64(changed),
		"interrupted_users":     interruptedUsers,
		"interrupted_nodes":     interruptedNodes,
		"quota_refresh_users":   quotaRefreshUsers,
		"quota_refresh_nodes":   quotaRefreshNodes,
		"quota_refresh_errors":  quotaRefreshErrs,
		"gpu_refresh_users":     gpuRefreshUsers,
		"gpu_refresh_nodes":     gpuRefreshNodes,
		"gpu_refresh_errors":    gpuRefreshErrs,
		"memory_refresh_users":  memoryRefreshUsers,
		"memory_refresh_nodes":  memoryRefreshNodes,
		"memory_refresh_errors": memoryRefreshErrs,
	})
}

func (s *Server) handleAdminPointsBatchSetUsers(c *gin.Context) {
	var req adminPointsBatchSetUsersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	var scopeErr error
	req.Scope, scopeErr = normalizePointsAdjustScope(req.Scope)
	req.NodeID = strings.TrimSpace(req.NodeID)
	if scopeErr != nil {
		c.JSON(400, gin.H{"error": scopeErr.Error()})
		return
	}
	if req.Scope == "node_exclusive" && req.NodeID == "" {
		c.JSON(400, gin.H{"error": "node_exclusive 模式下 node_id 不能为空"})
		return
	}
	if (req.Scope == "carryover" || req.Scope == "node_exclusive") && req.Value < 0 {
		c.JSON(400, gin.H{"error": "结转积分和节点专属积分不能设为负数"})
		return
	}

	targetUsernames := uniqTrim(req.Usernames)
	if len(targetUsernames) == 0 {
		c.JSON(400, gin.H{"error": "usernames 不能为空"})
		return
	}
	if len(targetUsernames) > 50000 {
		c.JSON(400, gin.H{"error": "批量用户数量过大（最大 50000）"})
		return
	}
	existingUsernames, err := s.store.FilterExistingPointsUsernames(c.Request.Context(), targetUsernames)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if len(existingUsernames) == 0 {
		c.JSON(400, gin.H{"error": "筛选结果为空或用户不存在"})
		return
	}

	maxOverdraftLimit := 0.0
	if req.Scope == "general" || req.Scope == "carryover" {
		maxOverdraftLimit, err = s.getMonthlyMaxOverdraftLimit(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	killThreshold := killAllProcessBalanceThreshold(maxOverdraftLimit)

	now := time.Now()
	changed := 0
	totalDelta := 0.0
	interruptUsers := map[string]struct{}{}
	stateRecheckUsers := map[string]float64{}
	method := "points_batch_filtered_set"
	if req.Scope == "carryover" {
		method = "points_batch_filtered_set_carry"
	} else if req.Scope == "node_exclusive" {
		method = "points_batch_filtered_set_node"
	}
	operator := s.currentOperator(c)
	actionCtx := withRechargeReason(c.Request.Context(), req.Reason)
	if err := s.store.WithTx(actionCtx, func(tx *sql.Tx) error {
		for _, username := range existingUsernames {
			if req.Scope == "node_exclusive" {
				_, delta, err := s.store.SetNodeExclusiveBalanceTx(
					actionCtx,
					tx,
					username,
					req.NodeID,
					req.Value,
					method,
					operator,
					s.cfg,
				)
				if err != nil {
					return err
				}
				totalDelta += delta
				if delta != 0 {
					changed++
				}
				continue
			}

			var res BalanceUpdateResult
			var delta float64
			var err error
			if req.Scope == "carryover" {
				res, delta, err = s.store.SetCarryoverBalanceTx(actionCtx, tx, username, req.Value, method, now, s.cfg)
			} else {
				res, delta, err = s.store.SetBalanceTx(actionCtx, tx, username, req.Value, method, now, s.cfg)
			}
			if err != nil {
				return err
			}
			totalDelta += delta
			if delta == 0 {
				continue
			}
			changed++
			stateRecheckUsers[username] = res.User.Balance + res.User.CarryoverBalance
			if shouldTriggerOverdraftInterrupt(res.PrevEffectiveBalance, res.EffectiveBalance, maxOverdraftLimit) {
				interruptUsers[username] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	interruptedUsers := 0
	interruptedNodes := 0
	if req.Scope == "general" || req.Scope == "carryover" {
		for username := range interruptUsers {
			n, err := s.enqueueHardInterruptForBillingUser(
				c.Request.Context(),
				username,
				maxOverdraftLimit,
				fmt.Sprintf("批量积分设定后余额低于 %.2f 积分，已超过每月最大欠费上限，强制中断", killThreshold),
			)
			if err != nil {
				log.Printf("筛选批量积分设定后强制中断失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				interruptedUsers++
				interruptedNodes += n
			}
		}
	}

	quotaRefreshUsers := 0
	quotaRefreshNodes := 0
	quotaRefreshErrs := 0
	gpuRefreshUsers := 0
	gpuRefreshNodes := 0
	gpuRefreshErrs := 0
	memoryRefreshUsers := 0
	memoryRefreshNodes := 0
	memoryRefreshErrs := 0
	if (req.Scope == "general" || req.Scope == "carryover") && len(stateRecheckUsers) > 0 {
		usernames := make([]string, 0, len(stateRecheckUsers))
		for username := range stateRecheckUsers {
			usernames = append(usernames, username)
		}
		sort.Strings(usernames)
		for _, username := range usernames {
			n, err := s.refreshCPUQuotaForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分设定后重算限速（%s）", username),
			)
			if err != nil {
				quotaRefreshErrs++
				log.Printf("筛选批量积分设定后刷新 CPU 限速失败 username=%s err=%v", username, err)
			} else if n > 0 {
				quotaRefreshUsers++
				quotaRefreshNodes += n
			}
			n, err = s.refreshGPUAccessForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分设定后同步 GPU 限制（%s）", username),
			)
			if err != nil {
				gpuRefreshErrs++
				log.Printf("筛选批量积分设定后刷新 GPU 限制失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				gpuRefreshUsers++
				gpuRefreshNodes += n
			}
			n, err = s.refreshMemoryLimitForBillingUser(
				c.Request.Context(),
				username,
				stateRecheckUsers[username],
				fmt.Sprintf("筛选批量积分设定后同步内存限额（%s）", username),
			)
			if err != nil {
				memoryRefreshErrs++
				log.Printf("筛选批量积分设定后刷新内存限额失败 username=%s err=%v", username, err)
				continue
			}
			if n > 0 {
				memoryRefreshUsers++
				memoryRefreshNodes += n
			}
		}
	}

	c.JSON(200, gin.H{
		"ok":                    true,
		"scope":                 req.Scope,
		"node_id":               req.NodeID,
		"value":                 req.Value,
		"requested_users":       len(targetUsernames),
		"matched_users":         len(existingUsernames),
		"changed_users":         changed,
		"unchanged_users":       len(existingUsernames) - changed,
		"skipped_users":         len(targetUsernames) - len(existingUsernames),
		"total_delta":           totalDelta,
		"interrupted_users":     interruptedUsers,
		"interrupted_nodes":     interruptedNodes,
		"quota_refresh_users":   quotaRefreshUsers,
		"quota_refresh_nodes":   quotaRefreshNodes,
		"quota_refresh_errors":  quotaRefreshErrs,
		"gpu_refresh_users":     gpuRefreshUsers,
		"gpu_refresh_nodes":     gpuRefreshNodes,
		"gpu_refresh_errors":    gpuRefreshErrs,
		"memory_refresh_users":  memoryRefreshUsers,
		"memory_refresh_nodes":  memoryRefreshNodes,
		"memory_refresh_errors": memoryRefreshErrs,
	})
}

func (s *Server) handleAdminPointsSpecialRulesList(c *gin.Context) {
	rows, err := s.store.ListSpecialMonthlyPointsRules(c.Request.Context(), 5000)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"rules": rows})
}

func (s *Server) handleAdminPointsSpecialRulesUpsert(c *gin.Context) {
	var req adminPointsSpecialRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Note = strings.TrimSpace(req.Note)
	if req.Username == "" {
		c.JSON(400, gin.H{"error": "username 不能为空"})
		return
	}
	if req.MonthlyPoints < 0 {
		c.JSON(400, gin.H{"error": "monthly_points 不能为负数"})
		return
	}
	if err := s.store.UpsertSpecialMonthlyPointsRule(c.Request.Context(), req.Username, req.MonthlyPoints, req.Enabled, req.Note, s.currentOperator(c)); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleAdminPointsSpecialRulesDelete(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(400, gin.H{"error": "username 不能为空"})
		return
	}
	if err := s.store.DeleteSpecialMonthlyPointsRule(c.Request.Context(), username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(404, gin.H{"error": "规则不存在"})
			return
		}
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleAdminPointsMonthlyConfigGet(c *gin.Context) {
	cfg, err := s.store.GetMonthlyPointsConfig(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"config": cfg})
}

func (s *Server) handleAdminPointsMonthlyConfigSet(c *gin.Context) {
	var req adminPointsMonthlyConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请求参数不合法"})
		return
	}
	if req.DoctorPoints < 0 || req.MasterPoints < 0 || req.OtherPoints < 0 || req.CarryoverLimit < 0 || req.MaxOverdraftLimit < 0 {
		c.JSON(400, gin.H{"error": "博士/硕士/其他积分、结转上限、每月最大欠费上限不能为负数"})
		return
	}
	if err := s.store.UpsertMonthlyPointsConfig(c.Request.Context(), MonthlyPointsConfig{
		DoctorPoints:      req.DoctorPoints,
		MasterPoints:      req.MasterPoints,
		OtherPoints:       req.OtherPoints,
		CarryoverLimit:    req.CarryoverLimit,
		MaxOverdraftLimit: req.MaxOverdraftLimit,
	}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) handleAdminPointsMonthlyResetStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	monthKey := strings.TrimSpace(c.Query("month"))
	if monthKey == "" {
		monthKey = time.Now().In(time.Local).Format("2006-01")
	}
	run, exists, err := s.store.GetMonthlyPointsResetRun(c.Request.Context(), monthKey)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	var runOut any = nil
	if exists {
		runOut = run
	}
	var latestOut any = nil
	if latest, ok, err := s.store.GetLatestMonthlyPointsResetRun(c.Request.Context()); err == nil && ok {
		latestOut = latest
	}
	c.JSON(200, gin.H{"month_key": monthKey, "has_run": exists, "run": runOut, "last_run": latestOut})
}

func (s *Server) handleAdminPointsMonthlyReset(c *gin.Context) {
	var req adminPointsMonthlyResetReq
	_ = c.ShouldBindJSON(&req)
	res, err := s.applyMonthlyPointsReset(c.Request.Context(), s.currentOperator(c), req.Force)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	msg := "月初积分重置已执行"
	if res.AlreadyRun {
		msg = "本月已执行过月初重置（如需重跑请勾选强制重置）"
	}
	c.JSON(200, gin.H{
		"ok":                   true,
		"message":              msg,
		"month_key":            res.MonthKey,
		"already_run":          res.AlreadyRun,
		"total_users":          res.TotalUsers,
		"changed_users":        res.ChangedUsers,
		"interrupted_users":    res.InterruptedUsers,
		"interrupted_nodes":    res.InterruptedTarget,
		"quota_refresh_users":  res.QuotaRefreshUsers,
		"quota_refresh_nodes":  res.QuotaRefreshNodes,
		"quota_refresh_errors": res.QuotaRefreshErrs,
		"run":                  res.Run,
	})
}
