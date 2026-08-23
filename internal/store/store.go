// Package store 提供基于 SQLite（modernc.org/sqlite 纯 Go 驱动）的
// 持久化层：建表迁移、各实体仓库与事务包装。数据落盘后
// 支持重启重新打开并恢复全部状态。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接与运行时信息。
type Store struct {
	db *sql.DB
}

// Querier 同时被 *sql.DB 与 *sql.Tx 满足，便于事务内复用仓库方法。
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx 是事务句柄的别名，业务包在 WithTx 回调中使用。
type Tx = sql.Tx

// Open 打开（或创建）SQLite 数据库并执行建表迁移。
// 空路径使用临时文件（smoke-test 场景）。
func Open(path string) (*Store, error) {
	if path == "" {
		path = fmt.Sprintf("file:keyproof-%d.db?_pragma=busy_timeout(5000)", time.Now().UnixNano())
	}
	dsn := path
	if _, err := sql.Open("sqlite", dsn); err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写者模型
	st := &Store{db: db}
	if err := st.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

// DB 暴露底层连接，供仓库方法使用。
func (s *Store) DB() *sql.DB { return s.db }

// Close 关闭数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// WithTx 在事务中执行 fn；fn 返回错误则回滚。
func (s *Store) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// migrate 建表。时间统一以 RFC3339 文本存储。
func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS keys (
	id        TEXT PRIMARY KEY,
	name      TEXT NOT NULL,
	kind      TEXT NOT NULL,
	status    TEXT NOT NULL,
	parent_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS subjects (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	status     TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS grants (
	subject_id TEXT NOT NULL,
	key_id     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (subject_id, key_id)
);
CREATE TABLE IF NOT EXISTS objects (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	status     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS wraps (
	object_id  TEXT NOT NULL,
	key_id     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (object_id, key_id)
);
CREATE TABLE IF NOT EXISTS plans (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	status     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS plan_steps (
	id           TEXT PRIMARY KEY,
	plan_id      TEXT NOT NULL,
	seq          INTEGER NOT NULL,
	action       TEXT NOT NULL,
	key_id       TEXT NOT NULL DEFAULT '',
	object_id    TEXT NOT NULL DEFAULT '',
	subject_id   TEXT NOT NULL DEFAULT '',
	target_key_id TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL,
	blocked_at   TEXT,
	created_at   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS proofs (
	id         TEXT PRIMARY KEY,
	plan_id    TEXT NOT NULL,
	step_id    TEXT NOT NULL,
	status     TEXT NOT NULL,
	digest     TEXT NOT NULL,
	input_fp   TEXT NOT NULL,
	summary    TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS executions (
	id          TEXT PRIMARY KEY,
	plan_id     TEXT NOT NULL,
	step_id     TEXT NOT NULL,
	action      TEXT NOT NULL,
	status      TEXT NOT NULL,
	reason      TEXT NOT NULL DEFAULT '',
	started_at  TEXT NOT NULL,
	finished_at TEXT
);
CREATE TABLE IF NOT EXISTS plan_locks (
	plan_id     TEXT NOT NULL,
	target_type TEXT NOT NULL,
	target_id   TEXT NOT NULL,
	PRIMARY KEY (target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_keys_parent ON keys(parent_id);
CREATE INDEX IF NOT EXISTS idx_grants_key ON grants(key_id);
CREATE INDEX IF NOT EXISTS idx_wraps_key ON wraps(key_id);
CREATE INDEX IF NOT EXISTS idx_steps_plan ON plan_steps(plan_id, seq);
CREATE INDEX IF NOT EXISTS idx_proofs_plan ON proofs(plan_id);
CREATE INDEX IF NOT EXISTS idx_exec_plan ON executions(plan_id);
`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Now 返回统一的 RFC3339 时间字符串。
func Now() string { return time.Now().UTC().Format(time.RFC3339) }
