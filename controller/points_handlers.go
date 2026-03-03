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

const hardInterruptBalanceThreshold = -10.0

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
}

type adminPointsSpecialRuleReq struct {
	Username      string  `json:"username"`
	MonthlyPoints float64 `json:"monthly_points"`
	Enabled       bool    `json:"enabled"`
}

type adminPointsMonthlyResetReq struct {
	Force bool `json:"force"`
}

type adminPointsMonthlyConfigReq struct {
	DoctorPoints   float64 `json:"doctor_points"`
	MasterPoints   float64 `json:"master_points"`
	OtherPoints    float64 `json:"other_points"`
	CarryoverLimit float64 `json:"carryover_limit"`
}

type pointsMonthlyResetSummary struct {
	MonthKey          string                 `json:"month_key"`
	AlreadyRun        bool                   `json:"already_run"`
	Run               *MonthlyPointsResetRun `json:"run,omitempty"`
	TotalUsers        int                    `json:"total_users"`
	ChangedUsers      int                    `json:"changed_users"`
	InterruptedUsers  int                    `json:"interrupted_users"`
	InterruptedTarget int                    `json:"interrupted_targets"`
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

func (s *Server) enqueueHardInterruptForBillingUser(ctx context.Context, billingUsername string, reason string) (int, error) {
	billingUsername = strings.TrimSpace(billingUsername)
	if billingUsername == "" {
		return 0, errors.New("billing_username 不能为空")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = fmt.Sprintf("余额低于 %.2f 积分，强制中断所有进程", hardInterruptBalanceThreshold)
	}

	targets := map[string]struct{}{}
	add := func(nodeID string, localUsername string) {
		nodeID = strings.TrimSpace(nodeID)
		localUsername = strings.TrimSpace(localUsername)
		if nodeID == "" || localUsername == "" {
			return
		}
		targets[nodeID+"|"+localUsername] = struct{}{}
	}

	accounts, err := s.store.ListUserNodeAccountsByBilling(ctx, billingUsername, 5000)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	for _, acc := range accounts {
		add(acc.NodeID, acc.LocalUsername)
	}

	// 没有映射时，按“平台用户名 == 节点本地用户名”兜底处理所有节点。
	if len(targets) == 0 {
		nodes, err := s.store.ListNodes(ctx, 5000)
		if err != nil {
			return 0, err
		}
		for _, n := range nodes {
			add(n.NodeID, billingUsername)
		}
	}

	if len(targets) == 0 {
		return 0, nil
	}

	keys := make([]string, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		nodeID := parts[0]
		localUsername := parts[1]
		s.enqueueNodeAction(nodeID, Action{
			Type:     "block_user",
			Username: localUsername,
			Reason:   reason,
		})
		s.enqueueNodeAction(nodeID, Action{
			Type:     "kill_all_processes",
			Username: localUsername,
			Reason:   reason,
		})
	}
	return len(keys), nil
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
	changedUsers := 0
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

		monthlyCfg, err := s.store.LoadMonthlyPointsConfigTx(ctx, tx)
		if err != nil {
			return err
		}
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
			}
			if res.User.Balance+res.User.CarryoverBalance <= hardInterruptBalanceThreshold {
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
	for username := range interruptUsers {
		n, err := s.enqueueHardInterruptForBillingUser(ctx, username, fmt.Sprintf("月初重置后余额低于 %.2f 积分，强制中断", hardInterruptBalanceThreshold))
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
	limit := parseLimit(c.Query("limit"), 1000, 5000)
	rows, err := s.store.ListPointsUsers(c.Request.Context(), keyword, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"users": rows})
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
	req.Scope = strings.TrimSpace(req.Scope)
	req.NodeID = strings.TrimSpace(req.NodeID)
	if req.Scope == "" {
		req.Scope = "general"
	}
	if req.Scope != "general" && req.Scope != "carryover" && req.Scope != "node_exclusive" {
		c.JSON(400, gin.H{"error": "scope 仅支持 general、carryover 或 node_exclusive"})
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
	var res BalanceUpdateResult
	carryoverBalance := 0.0
	exclusiveBalance := 0.0
	nodeExclusiveCurrent := 0.0
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		var err error
		if req.Scope == "node_exclusive" {
			row, rowErr := s.store.AdjustNodeExclusiveBalanceTx(
				c.Request.Context(),
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
			u, getErr := s.store.EnsureUserTx(c.Request.Context(), tx, req.Username, s.cfg.DefaultBalance)
			if getErr != nil {
				return getErr
			}
			exclusiveBalance, err = s.store.GetUserExclusiveBalanceTotalTx(c.Request.Context(), tx, req.Username)
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
			res, err = s.store.AdjustCarryoverBalanceTx(c.Request.Context(), tx, req.Username, req.Delta, method, now, s.cfg)
		} else {
			res, err = s.store.AdjustBalanceTx(c.Request.Context(), tx, req.Username, req.Delta, method, now, s.cfg)
		}
		if err != nil {
			return err
		}
		carryoverBalance = res.User.CarryoverBalance
		exclusiveBalance, err = s.store.GetUserExclusiveBalanceTotalTx(c.Request.Context(), tx, req.Username)
		return err
	}); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	interruptTargets := 0
	if (req.Scope == "general" || req.Scope == "carryover") && res.User.Balance+carryoverBalance <= hardInterruptBalanceThreshold {
		n, err := s.enqueueHardInterruptForBillingUser(
			c.Request.Context(),
			req.Username,
			fmt.Sprintf("管理员调整后余额低于 %.2f 积分，强制中断", hardInterruptBalanceThreshold),
		)
		if err != nil {
			c.JSON(500, gin.H{"error": "积分已调整，但下发强制中断失败: " + err.Error()})
			return
		}
		interruptTargets = n
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
	users, err := s.store.ListRegularUserProfilesForPoints(c.Request.Context(), 50000)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	changed := 0
	interruptUsers := map[string]struct{}{}
	method := "points_batch_plus"
	if req.Amount < 0 {
		method = "points_batch_minus"
	}
	if err := s.store.WithTx(c.Request.Context(), func(tx *sql.Tx) error {
		for _, u := range users {
			res, err := s.store.AdjustBalanceTx(c.Request.Context(), tx, u.Username, req.Amount, method, now, s.cfg)
			if err != nil {
				return err
			}
			changed++
			if res.User.Balance+res.User.CarryoverBalance <= hardInterruptBalanceThreshold {
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
	for username := range interruptUsers {
		n, err := s.enqueueHardInterruptForBillingUser(
			c.Request.Context(),
			username,
			fmt.Sprintf("全体积分调整后余额低于 %.2f 积分，强制中断", hardInterruptBalanceThreshold),
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

	c.JSON(200, gin.H{
		"ok":                true,
		"amount":            req.Amount,
		"granted_users":     changed,
		"adjusted_users":    changed,
		"total_granted":     req.Amount * float64(changed),
		"total_adjusted":    req.Amount * float64(changed),
		"interrupted_users": interruptedUsers,
		"interrupted_nodes": interruptedNodes,
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
	if req.Username == "" {
		c.JSON(400, gin.H{"error": "username 不能为空"})
		return
	}
	if req.MonthlyPoints < 0 {
		c.JSON(400, gin.H{"error": "monthly_points 不能为负数"})
		return
	}
	if err := s.store.UpsertSpecialMonthlyPointsRule(c.Request.Context(), req.Username, req.MonthlyPoints, req.Enabled, s.currentOperator(c)); err != nil {
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
	if req.DoctorPoints < 0 || req.MasterPoints < 0 || req.OtherPoints < 0 || req.CarryoverLimit < 0 {
		c.JSON(400, gin.H{"error": "博士/硕士/其他积分与结转上限不能为负数"})
		return
	}
	if err := s.store.UpsertMonthlyPointsConfig(c.Request.Context(), MonthlyPointsConfig{
		DoctorPoints:   req.DoctorPoints,
		MasterPoints:   req.MasterPoints,
		OtherPoints:    req.OtherPoints,
		CarryoverLimit: req.CarryoverLimit,
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
		"ok":                true,
		"message":           msg,
		"month_key":         res.MonthKey,
		"already_run":       res.AlreadyRun,
		"total_users":       res.TotalUsers,
		"changed_users":     res.ChangedUsers,
		"interrupted_users": res.InterruptedUsers,
		"interrupted_nodes": res.InterruptedTarget,
		"run":               res.Run,
	})
}
