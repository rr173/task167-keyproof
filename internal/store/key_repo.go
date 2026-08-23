package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
)

// KeyRepo 提供密钥的持久化操作。
type KeyRepo struct{}

// NewKeyRepo 构造密钥仓库。
func NewKeyRepo() *KeyRepo { return &KeyRepo{} }

const keyCols = `id, name, kind, status, parent_id, created_at, updated_at`

func scanKey(row interface{ Scan(dest ...any) error }) (*model.Key, error) {
	var k model.Key
	var parent, created, updated string
	if err := row.Scan(&k.ID, &k.Name, &k.Kind, &k.Status, &parent, &created, &updated); err != nil {
		return nil, err
	}
	k.ParentID = parent
	k.CreatedAt, _ = time.Parse(time.RFC3339, created)
	k.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &k, nil
}

// Create 插入密钥，重复 ID 返回 ErrConflict。
func (r *KeyRepo) Create(ctx context.Context, q Querier, k *model.Key) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO keys (id, name, kind, status, parent_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Name, string(k.Kind), string(k.Status), k.ParentID,
		Now(), Now())
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrConflict
		}
		return fmt.Errorf("insert key: %w", err)
	}
	return nil
}

// Get 按 ID 查询密钥，不存在返回 ErrNotFound。
func (r *KeyRepo) Get(ctx context.Context, q Querier, id string) (*model.Key, error) {
	row := q.QueryRowContext(ctx, `SELECT `+keyCols+` FROM keys WHERE id = ?`, id)
	k, err := scanKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get key %s: %w", id, err)
	}
	return k, nil
}

// List 列出全部密钥。
func (r *KeyRepo) List(ctx context.Context, q Querier) ([]*model.Key, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+keyCols+` FROM keys ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	defer rows.Close()
	var out []*model.Key
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateStatus 更新密钥状态与更新时间。
func (r *KeyRepo) UpdateStatus(ctx context.Context, q Querier, id string, status model.KeyStatus) error {
	res, err := q.ExecContext(ctx,
		`UPDATE keys SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), Now(), id)
	if err != nil {
		return fmt.Errorf("update key status: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// UpdateParent 调整密钥父节点（换层级用，需先做环检测）。
func (r *KeyRepo) UpdateParent(ctx context.Context, q Querier, id, parent string) error {
	res, err := q.ExecContext(ctx,
		`UPDATE keys SET parent_id = ?, updated_at = ? WHERE id = ?`,
		parent, Now(), id)
	if err != nil {
		return fmt.Errorf("update key parent: %w", err)
	}
	return requireAffected(res, model.ErrNotFound)
}

// CountByStatus 统计指定状态的密钥数量。
func (r *KeyRepo) CountByStatus(ctx context.Context, q Querier, status model.KeyStatus) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM keys WHERE status = ?`, string(status)).Scan(&n)
	return n, err
}
