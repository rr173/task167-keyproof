package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// ProofRepo 提供证明快照持久化。
type ProofRepo struct{}

// NewProofRepo 构造证明仓库。
func NewProofRepo() *ProofRepo { return &ProofRepo{} }

// Save 保存证明快照；同一 (plan_id, step_id) 已存在则更新。
func (r *ProofRepo) Save(ctx context.Context, q Querier, p *model.Proof) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO proofs (id, plan_id, step_id, status, digest, input_fp, summary, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   status = excluded.status, digest = excluded.digest,
		   input_fp = excluded.input_fp, summary = excluded.summary`,
		p.ID, p.PlanID, p.StepID, string(p.Status), p.Digest, p.InputFP, p.Summary, Now())
	if err != nil {
		return fmt.Errorf("save proof: %w", err)
	}
	return nil
}

// Get 查询证明快照。
func (r *ProofRepo) Get(ctx context.Context, q Querier, id string) (*model.Proof, error) {
	var p model.Proof
	var created string
	err := q.QueryRowContext(ctx,
		`SELECT id, plan_id, step_id, status, digest, input_fp, summary, created_at
		 FROM proofs WHERE id = ?`, id).
		Scan(&p.ID, &p.PlanID, &p.StepID, &p.Status, &p.Digest, &p.InputFP, &p.Summary, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get proof %s: %w", id, err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &p, nil
}

// GetByStep 查询某计划某步骤的证明快照（不存在返回 nil）。
func (r *ProofRepo) GetByStep(ctx context.Context, q Querier, planID, stepID string) (*model.Proof, error) {
	var p model.Proof
	var created string
	err := q.QueryRowContext(ctx,
		`SELECT id, plan_id, step_id, status, digest, input_fp, summary, created_at
		 FROM proofs WHERE plan_id = ? AND step_id = ?`, planID, stepID).
		Scan(&p.ID, &p.PlanID, &p.StepID, &p.Status, &p.Digest, &p.InputFP, &p.Summary, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get proof by step: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &p, nil
}

// List 列出计划的全部证明快照。
func (r *ProofRepo) List(ctx context.Context, q Querier, planID string) ([]*model.Proof, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, plan_id, step_id, status, digest, input_fp, summary, created_at
		 FROM proofs WHERE plan_id = ? ORDER BY created_at, id`, planID)
	if err != nil {
		return nil, fmt.Errorf("list proofs: %w", err)
	}
	defer rows.Close()
	var out []*model.Proof
	for rows.Next() {
		var p model.Proof
		var created string
		if err := rows.Scan(&p.ID, &p.PlanID, &p.StepID, &p.Status, &p.Digest, &p.InputFP, &p.Summary, &created); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ListAll 列出全部证明快照。
func (r *ProofRepo) ListAll(ctx context.Context, q Querier) ([]*model.Proof, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, plan_id, step_id, status, digest, input_fp, summary, created_at
		 FROM proofs ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list all proofs: %w", err)
	}
	defer rows.Close()
	var out []*model.Proof
	for rows.Next() {
		var p model.Proof
		var created string
		if err := rows.Scan(&p.ID, &p.PlanID, &p.StepID, &p.Status, &p.Digest, &p.InputFP, &p.Summary, &created); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, &p)
	}
	return out, rows.Err()
}
