package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	nodeAccountActionType       = "create_local_account"
	nodeActionLeaseDuration     = 60 * time.Second
	nodeActionDefaultMaxAttempt = 10
	nodeActionDispatchBatch     = 20
)

type nodeActionJobResult struct {
	Accepted bool
	Status   string
}

// ReconcileNodeAccountActionsFromSnapshot treats the node's authoritative
// local-user snapshot as a success signal only for identity-only actions. A
// public-key action requires an explicit callback because UID/GID alignment
// cannot prove that authorized_keys contains the newly issued key.
func (s *Store) ReconcileNodeAccountActionsFromSnapshot(ctx context.Context, nodeID string, localUsers []NodeLocalUser) (int, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || len(localUsers) == 0 {
		return 0, nil
	}
	type localIdentity struct {
		uid int
		gid int
	}
	identities := make(map[string]localIdentity, len(localUsers))
	for _, user := range localUsers {
		localUsername := strings.TrimSpace(user.LocalUsername)
		if localUsername == "" || user.UID == nil || user.PrimaryGID == nil || *user.UID <= 0 || *user.PrimaryGID <= 0 {
			continue
		}
		identities[localUsername] = localIdentity{uid: *user.UID, gid: *user.PrimaryGID}
	}
	if len(identities) == 0 {
		return 0, nil
	}
	reconciled := 0
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT job_id, local_username, payload::text
FROM node_action_jobs
WHERE node_id=$1
  AND action_type='create_local_account'
  AND status IN ('pending','leased')
FOR UPDATE`, nodeID)
		if err != nil {
			return err
		}
		type activeJob struct {
			jobID         int64
			localUsername string
			payload       string
		}
		jobs := make([]activeJob, 0)
		for rows.Next() {
			var job activeJob
			if err := rows.Scan(&job.jobID, &job.localUsername, &job.payload); err != nil {
				_ = rows.Close()
				return err
			}
			jobs = append(jobs, job)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, job := range jobs {
			identity, found := identities[strings.TrimSpace(job.localUsername)]
			if !found {
				continue
			}
			var action Action
			if err := json.Unmarshal([]byte(job.payload), &action); err != nil {
				continue
			}
			if strings.TrimSpace(action.PublicKey) != "" {
				continue
			}
			targetUID, targetGID := action.TargetUID, action.TargetPrimaryGID
			if targetGID <= 0 {
				targetGID = targetUID
			}
			if targetUID <= 0 || identity.uid != targetUID || identity.gid != targetGID {
				continue
			}
			res, err := tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='succeeded', lease_until=NULL, delivery_token='', last_error='', updated_at=NOW(), completed_at=NOW()
WHERE job_id=$1 AND status IN ('pending','leased')`, job.jobID)
			if err != nil {
				return err
			}
			if affected, err := res.RowsAffected(); err != nil {
				return err
			} else {
				reconciled += int(affected)
			}
		}
		return nil
	})
	return reconciled, err
}

func newNodeActionDeliveryToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// QueueNodeAccountAction persists the idempotent account creation/alignment action.
// Re-queuing the same node identity replaces the payload and invalidates an older
// delivery token. If the older action is still leased, the replacement waits for
// that lease to expire before it can be delivered, preventing an old key write
// from racing a newly generated key.
func (s *Store) QueueNodeAccountAction(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	action Action,
) (int64, error) {
	var jobID int64
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		jobID, err = s.queueNodeAccountActionTx(ctx, tx, nodeID, localUsername, billingUsername, action)
		return err
	})
	return jobID, err
}

func (s *Store) queueNodeAccountActionTx(
	ctx context.Context,
	tx *sql.Tx,
	nodeID string,
	localUsername string,
	billingUsername string,
	action Action,
) (int64, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return 0, errors.New("node_id/local_username/billing_username 不能为空")
	}
	if strings.TrimSpace(action.Type) != nodeAccountActionType {
		return 0, fmt.Errorf("不支持持久化的节点动作类型: %s", action.Type)
	}
	if strings.TrimSpace(action.Username) != localUsername {
		return 0, errors.New("action.username 与 local_username 不一致")
	}
	action.ActionID = 0
	action.ActionToken = ""
	payload, err := json.Marshal(action)
	if err != nil {
		return 0, err
	}
	token, err := newNodeActionDeliveryToken()
	if err != nil {
		return 0, err
	}
	var jobID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO node_action_jobs(
  node_id, local_username, billing_username, action_type, payload,
  status, attempt_count, max_attempts, next_attempt_at, lease_until,
  delivery_token, last_error, created_at, updated_at, completed_at
)
VALUES($1,$2,$3,$4,$5::jsonb,'pending',0,$6,NOW(),NULL,$7,'',NOW(),NOW(),NULL)
ON CONFLICT (node_id, local_username)
  WHERE action_type = 'create_local_account'
    AND status IN ('pending', 'leased')
DO UPDATE SET
  billing_username=EXCLUDED.billing_username,
  payload=EXCLUDED.payload,
  status='pending',
  attempt_count=0,
  max_attempts=EXCLUDED.max_attempts,
  next_attempt_at=CASE
    WHEN node_action_jobs.status='leased'
     AND node_action_jobs.lease_until IS NOT NULL
     AND node_action_jobs.lease_until > NOW()
      THEN node_action_jobs.lease_until
    ELSE NOW()
  END,
  lease_until=NULL,
  delivery_token=EXCLUDED.delivery_token,
  last_error='',
  updated_at=NOW(),
  completed_at=NULL
RETURNING job_id`,
		nodeID,
		localUsername,
		billingUsername,
		nodeAccountActionType,
		string(payload),
		nodeActionDefaultMaxAttempt,
		token,
	).Scan(&jobID)
	return jobID, err
}

// UpsertUserNodeAccountAndQueueActionWithAudit commits the mapping and its
// account action atomically. A successful provisioning response therefore
// cannot leave a mapping with no executable public-key action behind.
func (s *Store) UpsertUserNodeAccountAndQueueActionWithAudit(
	ctx context.Context,
	nodeID string,
	localUsername string,
	billingUsername string,
	operator string,
	source string,
	reason string,
	action Action,
) (int64, error) {
	nodeID = strings.TrimSpace(nodeID)
	localUsername = strings.TrimSpace(localUsername)
	billingUsername = strings.TrimSpace(billingUsername)
	if nodeID == "" || localUsername == "" || billingUsername == "" {
		return 0, errors.New("node_id/local_username/billing_username 不能为空")
	}
	var jobID int64
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		oldBilling, existed, err := s.getUserNodeAccountBillingTx(ctx, tx, nodeID, localUsername)
		if err != nil {
			return err
		}
		if existed && oldBilling != billingUsername {
			return &NodeAccountOwnershipConflictError{
				NodeID:          nodeID,
				LocalUsername:   localUsername,
				ExistingBilling: oldBilling,
				RequestedBy:     billingUsername,
			}
		}
		if err := s.UpsertUserNodeAccountTx(ctx, tx, nodeID, localUsername, billingUsername); err != nil {
			return err
		}
		if !existed {
			if err := s.insertUserNodeAccountAuditTx(
				ctx, tx, nodeID, localUsername, oldBilling, billingUsername,
				"mapping_create", operator, source, reason,
			); err != nil {
				return err
			}
		}
		jobID, err = s.queueNodeAccountActionTx(ctx, tx, nodeID, localUsername, billingUsername, action)
		return err
	})
	return jobID, err
}

func (s *Store) UpdateUserNodeAccountAndQueueActionWithAudit(
	ctx context.Context,
	oldNodeID string,
	oldLocalUsername string,
	oldBillingUsername string,
	newNodeID string,
	newLocalUsername string,
	newBillingUsername string,
	operator string,
	source string,
	reason string,
	action Action,
) (int64, error) {
	var jobID int64
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.updateUserNodeAccountWithAuditTx(
			ctx, tx,
			oldNodeID, oldLocalUsername, oldBillingUsername,
			newNodeID, newLocalUsername, newBillingUsername,
			operator, source, reason,
		); err != nil {
			return err
		}
		if strings.TrimSpace(oldNodeID) != strings.TrimSpace(newNodeID) ||
			strings.TrimSpace(oldLocalUsername) != strings.TrimSpace(newLocalUsername) ||
			strings.TrimSpace(oldBillingUsername) != strings.TrimSpace(newBillingUsername) {
			if _, err := tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='cancelled', lease_until=NULL, delivery_token='', last_error='账号映射已改到新的节点身份', updated_at=NOW(), completed_at=NOW()
WHERE node_id=$1 AND local_username=$2 AND billing_username=$3
  AND status IN ('pending','leased')`, oldNodeID, oldLocalUsername, oldBillingUsername); err != nil {
				return err
			}
		}
		var err error
		jobID, err = s.queueNodeAccountActionTx(
			ctx, tx, newNodeID, newLocalUsername, newBillingUsername, action,
		)
		return err
	})
	return jobID, err
}

func (s *Store) LeaseNodeAccountActions(ctx context.Context, nodeID string, limit int) ([]Action, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if limit <= 0 || limit > nodeActionDispatchBatch {
		limit = nodeActionDispatchBatch
	}
	leased := make([]Action, 0, limit)
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='failed',
    lease_until=NULL,
    delivery_token='',
    last_error=CASE
      WHEN TRIM(last_error)='' THEN '节点动作多次投递后仍未收到成功回执'
      ELSE last_error
    END,
    updated_at=NOW(),
    completed_at=NOW()
WHERE node_id=$1
  AND action_type='create_local_account'
  AND attempt_count >= max_attempts
  AND (
    (status='pending' AND next_attempt_at <= NOW())
    OR (status='leased' AND lease_until IS NOT NULL AND lease_until <= NOW())
  )`, nodeID); err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT naj.job_id, naj.payload::text
FROM node_action_jobs naj
JOIN user_node_accounts una
  ON una.node_id=naj.node_id
 AND una.local_username=naj.local_username
 AND una.billing_username=naj.billing_username
WHERE naj.node_id=$1
  AND naj.action_type='create_local_account'
  AND naj.attempt_count < naj.max_attempts
  AND (
    (naj.status='pending' AND naj.next_attempt_at <= NOW())
    OR (naj.status='leased' AND naj.lease_until IS NOT NULL AND naj.lease_until <= NOW())
  )
ORDER BY naj.job_id
LIMIT $2
FOR UPDATE OF naj SKIP LOCKED`, nodeID, limit)
		if err != nil {
			return err
		}
		type queued struct {
			jobID   int64
			payload string
		}
		jobs := make([]queued, 0, limit)
		for rows.Next() {
			var job queued
			if err := rows.Scan(&job.jobID, &job.payload); err != nil {
				_ = rows.Close()
				return err
			}
			jobs = append(jobs, job)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, job := range jobs {
			var action Action
			if err := json.Unmarshal([]byte(job.payload), &action); err != nil {
				if _, updateErr := tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='failed', last_error=$2, delivery_token='', lease_until=NULL, updated_at=NOW(), completed_at=NOW()
WHERE job_id=$1`, job.jobID, "持久化动作内容损坏: "+err.Error()); updateErr != nil {
					return updateErr
				}
				continue
			}
			token, err := newNodeActionDeliveryToken()
			if err != nil {
				return err
			}
			res, err := tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='leased',
    attempt_count=attempt_count+1,
    lease_until=NOW()+($2 * INTERVAL '1 second'),
    delivery_token=$3,
    updated_at=NOW()
WHERE job_id=$1`, job.jobID, int(nodeActionLeaseDuration.Seconds()), token)
			if err != nil {
				return err
			}
			if affected, err := res.RowsAffected(); err != nil {
				return err
			} else if affected != 1 {
				return fmt.Errorf("租约更新失败: job_id=%d affected=%d", job.jobID, affected)
			}
			action.ActionID = job.jobID
			action.ActionToken = token
			leased = append(leased, action)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return leased, nil
}

func (s *Store) CompleteNodeAccountAction(
	ctx context.Context,
	jobID int64,
	nodeID string,
	deliveryToken string,
	success bool,
	resultError string,
) (nodeActionJobResult, error) {
	nodeID = strings.TrimSpace(nodeID)
	deliveryToken = strings.TrimSpace(deliveryToken)
	resultError = strings.TrimSpace(resultError)
	if len(resultError) > 4000 {
		resultError = resultError[:4000]
	}
	if jobID <= 0 || nodeID == "" || deliveryToken == "" {
		return nodeActionJobResult{}, errors.New("action_id/node_id/action_token 不能为空")
	}
	result := nodeActionJobResult{}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		var storedToken string
		var attempts int
		var maxAttempts int
		err := tx.QueryRowContext(ctx, `
SELECT status, delivery_token, attempt_count, max_attempts
FROM node_action_jobs
WHERE job_id=$1 AND node_id=$2
FOR UPDATE`, jobID, nodeID).Scan(&status, &storedToken, &attempts, &maxAttempts)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result.Status = strings.TrimSpace(status)
		if status != "leased" || storedToken != deliveryToken {
			return nil
		}
		result.Accepted = true
		if success {
			_, err = tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='succeeded', lease_until=NULL, delivery_token='', last_error='', updated_at=NOW(), completed_at=NOW()
WHERE job_id=$1`, jobID)
			result.Status = "succeeded"
			return err
		}
		if resultError == "" {
			resultError = "节点执行动作失败，未返回具体错误"
		}
		if attempts >= maxAttempts {
			_, err = tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='failed', lease_until=NULL, delivery_token='', last_error=$2, updated_at=NOW(), completed_at=NOW()
WHERE job_id=$1`, jobID, resultError)
			result.Status = "failed"
			return err
		}
		retrySeconds := attempts * 3
		if retrySeconds < 3 {
			retrySeconds = 3
		}
		if retrySeconds > 30 {
			retrySeconds = 30
		}
		_, err = tx.ExecContext(ctx, `
UPDATE node_action_jobs
SET status='pending',
    next_attempt_at=NOW()+($2 * INTERVAL '1 second'),
    lease_until=NULL,
    delivery_token='',
    last_error=$3,
    updated_at=NOW()
WHERE job_id=$1`, jobID, retrySeconds, resultError)
		result.Status = "pending"
		return err
	})
	return result, err
}
