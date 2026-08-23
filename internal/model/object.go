package model

import "time"

// SubjectStatus 主体的生命周期状态。
type SubjectStatus string

const (
	SubjectActive  SubjectStatus = "active"  // 可用
	SubjectRemoved SubjectStatus = "removed" // 已移除：不可新增授权
)

// Valid 校验主体状态是否合法。
func (s SubjectStatus) Valid() bool {
	return s == SubjectActive || s == SubjectRemoved
}

// Subject 表示一个有权解密的租户/工程师主体。
type Subject struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Status    SubjectStatus `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
}

// Grant 表示主体 -> 密钥的授权边：主体可解密封装在该密钥
// 及其全部子孙密钥下的对象（授权沿层级向下传播）。
type Grant struct {
	SubjectID string    `json:"subjectId"`
	KeyID     string    `json:"keyId"`
	CreatedAt time.Time `json:"createdAt"`
}

// ObjectStatus 加密对象生命周期状态机：
//
//	protected -> migrating -> rewrapped（迁移成功完成）
//	protected -> orphaned（唯一可解密密钥被吊销/退休且无替代）
//	* -> rewrapped（任一重封装成功后落于该终态，证明与恢复据此识别完成）
type ObjectStatus string

const (
	ObjectProtected ObjectStatus = "protected" // 受保护：存在可解密路径
	ObjectMigrating ObjectStatus = "migrating" // 迁移中：正在重新封装（多阶段未完成）
	ObjectRewrapped ObjectStatus = "rewrapped" // 已重新封装：迁移完成，证明与恢复可据此识别
	ObjectOrphaned  ObjectStatus = "orphaned"  // 孤立：无可解密路径
)

// Valid 校验对象状态是否合法。
func (s ObjectStatus) Valid() bool {
	switch s {
	case ObjectProtected, ObjectMigrating, ObjectRewrapped, ObjectOrphaned:
		return true
	default:
		return false
	}
}

// Rewrapped 表示对象已完成重新封装（正/反向重封装均落于该终态）。
func (s ObjectStatus) Rewrapped() bool { return s == ObjectRewrapped }

// Object 表示一个被密钥加密的受保护对象。
type Object struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Status    ObjectStatus `json:"status"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// Wrap 表示对象 -> 密钥的封装边：对象被该密钥加密。
type Wrap struct {
	ObjectID  string    `json:"objectId"`
	KeyID     string    `json:"keyId"`
	CreatedAt time.Time `json:"createdAt"`
}
