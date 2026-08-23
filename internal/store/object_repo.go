package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// ObjectRepo 提供加密对象与封装边持久化。
type ObjectRepo struct{}

// NewObjectRepo 构造对象仓库。
func NewObjectRepo() *ObjectRepo { return &ObjectRepo{} }

// Create 登记加密对象。
func (r *ObjectRepo) Create(ctx context.Context, q Querier, o *model.Object) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO objects (id, name, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		o.ID, o.Name, string(o.Status), Now(), Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert object: %w", err)
	}
	return nil
}

// Get 查询对象。
func (r *ObjectRepo) Get(ctx context.Context, q Querier, id string) (*model.Object, error) {
	var o model.Object
	var created, updated string
	err := q.QueryRowContext(ctx,
		`SELECT id, name, status, created_at, updated_at FROM objects WHERE id = ?`, id).
		Scan(&o.ID, &o.Name, &o.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", id, err)
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339, created)
	o.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &o, nil
}

// List 列出全部对象。
func (r *ObjectRepo) List(ctx context.Context, q Querier) ([]*model.Object, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, status, created_at, updated_at FROM objects ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	defer rows.Close()
	var out []*model.Object
	for rows.Next() {
		var o model.Object
		var created, updated string
		if err := rows.Scan(&o.ID, &o.Name, &o.Status, &created, &updated); err != nil {
			return nil, err
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339, created)
		o.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, &o)
	}
	return out, rows.Err()
}

// UpdateStatus 更新对象状态。
func (r *ObjectRepo) UpdateStatus(ctx context.Context, q Querier, id string, status model.ObjectStatus) error {
	res, err := q.ExecContext(ctx,
		`UPDATE objects SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), Now(), id)
	if err != nil {
		return fmt.Errorf("update object status: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// AddWrap 新增封装边（对象 -> 密钥），重复返回 ErrConflict。
func (r *ObjectRepo) AddWrap(ctx context.Context, q Querier, w *model.Wrap) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO wraps (object_id, key_id, created_at) VALUES (?, ?, ?)`,
		w.ObjectID, w.KeyID, Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert wrap: %w", err)
	}
	return nil
}

// RemoveWrap 移除封装边。
func (r *ObjectRepo) RemoveWrap(ctx context.Context, q Querier, objectID, keyID string) error {
	res, err := q.ExecContext(ctx,
		`DELETE FROM wraps WHERE object_id = ? AND key_id = ?`, objectID, keyID)
	if err != nil {
		return fmt.Errorf("remove wrap: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// ListWraps 列出封装边；可按对象或密钥过滤（为空则不过滤）。
func (r *ObjectRepo) ListWraps(ctx context.Context, q Querier, objectID, keyID string) ([]*model.Wrap, error) {
	query := `SELECT object_id, key_id, created_at FROM wraps`
	var args []any
	if objectID != "" {
		query += ` WHERE object_id = ?`
		args = append(args, objectID)
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
		return nil, fmt.Errorf("list wraps: %w", err)
	}
	defer rows.Close()
	var out []*model.Wrap
	for rows.Next() {
		var w model.Wrap
		var created string
		if err := rows.Scan(&w.ObjectID, &w.KeyID, &created); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, &w)
	}
	return out, rows.Err()
}

// WrapCount 统计封装边数量。
func (r *ObjectRepo) WrapCount(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM wraps`).Scan(&n)
	return n, err
}

// WrapKeyIDs 返回对象全部封装密钥 ID。
func (r *ObjectRepo) WrapKeyIDs(ctx context.Context, q Querier, objectID string) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT key_id FROM wraps WHERE object_id = ? ORDER BY created_at`, objectID)
	if err != nil {
		return nil, fmt.Errorf("list wrap keys: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
