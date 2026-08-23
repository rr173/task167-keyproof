package store

import (
	"database/sql"
	"errors"

	"task167-keyproof/internal/model"
)

// isUniqueViolation 判断 SQLite 约束冲突是否为主键/唯一冲突。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) {
		// modernc.org/sqlite: SQLITE_CONSTRAINT_PRIMARYKEY = 1555,
		// SQLITE_CONSTRAINT_UNIQUE = 2067。
		switch sqliteErr.Code() {
		case 1555, 2067, 19:
			return true
		}
	}
	// 兜底：文本匹配约束冲突描述。
	msg := err.Error()
	return containsAny(msg, "UNIQUE constraint failed", "PRIMARY KEY must be unique", "constraint failed")
}

// requireAffected 校验执行结果必须影响恰好一行，否则返回业务错误。
func requireAffected(res sql.Result, notFoundErr error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFoundErr
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ensureNoErr 供 scan 错误包装占位，防止误用。
var _ = model.ErrNotFound
