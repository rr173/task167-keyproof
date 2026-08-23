package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// ExecutionRepo 提供执行记录与跨计划锁持久化。
type ExecutionRepo struct{}

// NewExecutionRepo 构造执行记录仓库。
func NewExecutionRepo() *ExecutionRepo { return &ExecutionRepo{} }

// Insert 插入一条执行记录（开始）。
func (r *ExecutionRepo) Insert(ctx context.Context, q Querier, e *model.Execution) error {
	var finished any
	if e.FinishedAt != nil {
		finished = e.FinishedAt.UTC().Format(time.RFC3339)
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO executions (id, plan_id, step_id, action, status, reason, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.PlanID, e.StepID, string(e.Action), string(e.Status), e.Reason,
		e.StartedAt.UTC().Format(time.RFC3339), finished)
	if err != nil {
		return fmt.Errorf("insert execution: %w", err)
	}
	return nil
}

// Finish 更新执行记录终态。
func (r *ExecutionRepo) Finish(ctx context.Context, q Querier, id string, status model.ExecStatus, reason string) error {
	var finished any = Now()
	res, err := q.ExecContext(ctx,
		`UPDATE executions SET status = ?, reason = ?, finished_at = ? WHERE id = ?`,
		string(status), reason, finished, id)
	if err != nil {
		return fmt.Errorf("finish execution: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// ListByPlan 列出计划的执行记录。
func (r *ExecutionRepo) ListByPlan(ctx context.Context, q Querier, planID string) ([]*model.Execution, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, plan_id, step_id, action, status, reason, started_at, finished_at
		 FROM executions WHERE plan_id = ? ORDER BY started_at, id`, planID)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()
	var out []*model.Execution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListAll 列出全部执行记录。
func (r *ExecutionRepo) ListAll(ctx context.Context, q Querier) ([]*model.Execution, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, plan_id, step_id, action, status, reason, started_at, finished_at
		 FROM executions ORDER BY started_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list all executions: %w", err)
	}
	defer rows.Close()
	var out []*model.Execution
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanExecution(row interface{ Scan(dest ...any) error }) (*model.Execution, error) {
	var e model.Execution
	var started, finished string
	var finishedNull sql.NullString
	if err := row.Scan(&e.ID, &e.PlanID, &e.StepID, &e.Action, &e.Status, &e.Reason, &started, &finishedNull); err != nil {
		return nil, err
	}
	e.StartedAt, _ = time.Parse(time.RFC3339, started)
	if finishedNull.Valid {
		t, _ := time.Parse(time.RFC3339, finishedNull.String)
		e.FinishedAt = &t
	}
	_ = finished
	return &e, nil
}

// LastExecution 返回计划最后一次执行记录（按开始时间倒序），不存在返回 nil。
func (r *ExecutionRepo) LastExecution(ctx context.Context, q Querier, planID string) (*model.Execution, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, plan_id, step_id, action, status, reason, started_at, finished_at
		 FROM executions WHERE plan_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`, planID)
	e, err := scanExecution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("last execution: %w", err)
	}
	return e, nil
}

// AcquireLock 获取目标锁；被其它计划占用返回 ErrCrossPlan。
func (r *ExecutionRepo) AcquireLock(ctx context.Context, q Querier, planID, targetType, targetID string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO plan_locks (plan_id, target_type, target_id) VALUES (?, ?, ?)`,
		planID, targetType, targetID)
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrCrossPlan
		}
		return fmt.Errorf("acquire lock: %w", err)
	}
	return nil
}

// ReleaseLock 释放目标锁（仅释放属于该计划的锁）。
func (r *ExecutionRepo) ReleaseLock(ctx context.Context, q Querier, planID, targetType, targetID string) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM plan_locks WHERE plan_id = ? AND target_type = ? AND target_id = ?`,
		planID, targetType, targetID)
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

// ReleasePlanLocks 释放计划占用的全部锁。
func (r *ExecutionRepo) ReleasePlanLocks(ctx context.Context, q Querier, planID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM plan_locks WHERE plan_id = ?`, planID)
	if err != nil {
		return fmt.Errorf("release plan locks: %w", err)
	}
	return nil
}

// LocksOfPlan 返回计划当前占用锁。
func (r *ExecutionRepo) LocksOfPlan(ctx context.Context, q Querier, planID string) ([]*model.PlanLock, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT plan_id, target_type, target_id FROM plan_locks WHERE plan_id = ?`, planID)
	if err != nil {
		return nil, fmt.Errorf("locks of plan: %w", err)
	}
	defer rows.Close()
	var out []*model.PlanLock
	for rows.Next() {
		var l model.PlanLock
		if err := rows.Scan(&l.PlanID, &l.TargetType, &l.TargetID); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}
