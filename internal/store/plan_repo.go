package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// PlanRepo 提供轮换计划与步骤持久化。
type PlanRepo struct{}

// NewPlanRepo 构造计划仓库。
func NewPlanRepo() *PlanRepo { return &PlanRepo{} }

// CreatePlan 创建计划。
func (r *PlanRepo) CreatePlan(ctx context.Context, q Querier, p *model.Plan) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO plans (id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(p.Status), Now(), Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert plan: %w", err)
	}
	return nil
}

// GetPlan 查询计划。
func (r *PlanRepo) GetPlan(ctx context.Context, q Querier, id string) (*model.Plan, error) {
	var p model.Plan
	var created, updated string
	err := q.QueryRowContext(ctx,
		`SELECT id, name, status, created_at, updated_at FROM plans WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plan %s: %w", id, err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &p, nil
}

// ListPlans 列出全部计划。
func (r *PlanRepo) ListPlans(ctx context.Context, q Querier) ([]*model.Plan, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, status, created_at, updated_at FROM plans ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	var out []*model.Plan
	for rows.Next() {
		var p model.Plan
		var created, updated string
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &created, &updated); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, &p)
	}
	return out, rows.Err()
}

// UpdatePlanStatus 更新计划状态。
func (r *PlanRepo) UpdatePlanStatus(ctx context.Context, q Querier, id string, status model.PlanStatus) error {
	res, err := q.ExecContext(ctx,
		`UPDATE plans SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), Now(), id)
	if err != nil {
		return fmt.Errorf("update plan status: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// AddStep 追加计划步骤，seq 由调用方提供。
func (r *PlanRepo) AddStep(ctx context.Context, q Querier, s *model.PlanStep) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO plan_steps (id, plan_id, seq, action, key_id, object_id, subject_id, target_key_id, status, blocked_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.PlanID, s.Seq, string(s.Action), s.KeyID, s.ObjectID, s.SubjectID,
		s.TargetKeyID, string(s.Status), nil, Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert plan step: %w", err)
	}
	return nil
}

// ListSteps 按序号升序列出计划步骤。
func (r *PlanRepo) ListSteps(ctx context.Context, q Querier, planID string) ([]*model.PlanStep, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, plan_id, seq, action, key_id, object_id, subject_id, target_key_id, status, blocked_at, created_at
		 FROM plan_steps WHERE plan_id = ? ORDER BY seq`, planID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	defer rows.Close()
	return scanSteps(rows)
}

func scanSteps(rows *sql.Rows) ([]*model.PlanStep, error) {
	var out []*model.PlanStep
	for rows.Next() {
		var s model.PlanStep
		var blocked *string
		var created string
		if err := rows.Scan(&s.ID, &s.PlanID, &s.Seq, &s.Action, &s.KeyID, &s.ObjectID,
			&s.SubjectID, &s.TargetKeyID, &s.Status, &blocked, &created); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if blocked != nil {
			t, _ := time.Parse(time.RFC3339, *blocked)
			s.BlockedAt = &t
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// UpdateStepStatus 更新步骤状态与阻断时间。
//
// blocked_at 仅在步骤进入 blocked 态时有意义；离开 blocked 态
// （verified/applied/rolled_back）时必须清空，否则一个已验证的
// 步骤仍残留阻断时间戳，与“已验证”语义冲突，并阻碍修复后
// 的重新验证把步骤恢复为可执行、已验证状态。
// blockedAt 非空时以其为准；为 nil 且目标为 blocked 时记当前时间；
// 为 nil 且目标为其它状态时清空。
func (r *PlanRepo) UpdateStepStatus(ctx context.Context, q Querier, stepID string, status model.StepStatus, blockedAt *time.Time) error {
	var blocked any
	switch {
	case blockedAt != nil:
		blocked = blockedAt.UTC().Format(time.RFC3339)
	case status == model.StepBlocked:
		blocked = Now()
	default:
		blocked = nil
	}
	res, err := q.ExecContext(ctx,
		`UPDATE plan_steps SET status = ?, blocked_at = ? WHERE id = ?`,
		string(status), blocked, stepID)
	if err != nil {
		return fmt.Errorf("update step status: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// AppliedCount 统计计划已应用步骤数。
func (r *PlanRepo) AppliedCount(ctx context.Context, q Querier, planID string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plan_steps WHERE plan_id = ? AND status = ?`,
		planID, string(model.StepApplied)).Scan(&n)
	return n, err
}

// NextSeq 返回计划下一条步骤序号。
func (r *PlanRepo) NextSeq(ctx context.Context, q Querier, planID string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM plan_steps WHERE plan_id = ?`, planID).Scan(&n)
	return n, err
}
