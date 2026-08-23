package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// SubjectRepo 提供主体与授权边持久化。
type SubjectRepo struct{}

// NewSubjectRepo 构造主体仓库。
func NewSubjectRepo() *SubjectRepo { return &SubjectRepo{} }

// Create 登记主体。
func (r *SubjectRepo) Create(ctx context.Context, q Querier, s *model.Subject) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO subjects (id, name, status, created_at) VALUES (?, ?, ?, ?)`,
		s.ID, s.Name, string(s.Status), Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert subject: %w", err)
	}
	return nil
}

// Get 查询主体。
func (r *SubjectRepo) Get(ctx context.Context, q Querier, id string) (*model.Subject, error) {
	var s model.Subject
	var created string
	err := q.QueryRowContext(ctx,
		`SELECT id, name, status, created_at FROM subjects WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Status, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subject %s: %w", id, err)
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &s, nil
}

// UpdateStatus 更新主体生命周期状态。
// 移除（removed）后主体不再具备授权与解密能力，但其历史授权边保留，
// 以供审计与退休残留检测。
func (r *SubjectRepo) UpdateStatus(ctx context.Context, q Querier, id string, status model.SubjectStatus) error {
	res, err := q.ExecContext(ctx,
		`UPDATE subjects SET status = ? WHERE id = ?`, string(status), id)
	if err != nil {
		return fmt.Errorf("update subject status: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// List 列出全部主体。
func (r *SubjectRepo) List(ctx context.Context, q Querier) ([]*model.Subject, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, status, created_at FROM subjects ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list subjects: %w", err)
	}
	defer rows.Close()
	var out []*model.Subject
	for rows.Next() {
		var s model.Subject
		var created string
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &created); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, &s)
	}
	return out, rows.Err()
}

// AddGrant 新增授权边，重复返回 ErrConflict。
func (r *SubjectRepo) AddGrant(ctx context.Context, q Querier, g *model.Grant) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO grants (subject_id, key_id, created_at) VALUES (?, ?, ?)`,
		g.SubjectID, g.KeyID, Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert grant: %w", err)
	}
	return nil
}

// RemoveGrant 撤销授权边，不存在返回 ErrNotFound。
func (r *SubjectRepo) RemoveGrant(ctx context.Context, q Querier, subjectID, keyID string) error {
	res, err := q.ExecContext(ctx,
		`DELETE FROM grants WHERE subject_id = ? AND key_id = ?`, subjectID, keyID)
	if err != nil {
		return fmt.Errorf("remove grant: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// ListGrants 列出授权边；可按主体或密钥过滤（为空则不过滤）。
func (r *SubjectRepo) ListGrants(ctx context.Context, q Querier, subjectID, keyID string) ([]*model.Grant, error) {
	query := `SELECT subject_id, key_id, created_at FROM grants`
	var args []any
	if subjectID != "" {
		query += ` WHERE subject_id = ?`
		args = append(args, subjectID)
		if keyID != "" {
			query += ` AND key_id = ?`
			args = append(args, keyID)
		}
	} else if keyID != "" {
		query += ` WHERE key_id = ?`
		args = append(args, keyID)
	}
	query += ` ORDER BY created_at`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer rows.Close()
	var out []*model.Grant
	for rows.Next() {
		var g model.Grant
		var created string
		if err := rows.Scan(&g.SubjectID, &g.KeyID, &created); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, &g)
	}
	return out, rows.Err()
}

// GrantCount 统计授权边数量。
func (r *SubjectRepo) GrantCount(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM grants`).Scan(&n)
	return n, err
}
