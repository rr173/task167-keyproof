// Package model 定义多方密钥轮换覆盖证明服务的领域实体、
// 状态机与错误码。所有业务包共享本包，避免循环依赖。
package model

import "time"

// KeyKind 密钥在层级中的角色：根密钥 -> 租户密钥 -> 数据密钥。
type KeyKind string

const (
	KindRoot   KeyKind = "root"   // 根密钥：无父密钥，必须存在授权主体
	KindTenant KeyKind = "tenant" // 租户密钥：父为根密钥或租户密钥
	KindData   KeyKind = "data"   // 数据密钥：实际封装加密对象的密钥
)

// Valid 校验密钥种类是否合法。
func (k KeyKind) Valid() bool {
	switch k {
	case KindRoot, KindTenant, KindData:
		return true
	default:
		return false
	}
}

// KeyStatus 密钥生命周期状态机：
//
//	candidate -> active -> retiring -> retired
//	candidate -> active -> revoking -> revoked
//	candidate -> revoked
type KeyStatus string

const (
	KeyCandidate KeyStatus = "candidate" // 候选：已登记但未启用，不能被引用
	KeyActive    KeyStatus = "active"    // 启用：可被授权与封装引用
	KeyRetiring  KeyStatus = "retiring"  // 退役待清理：等待引用清空
	KeyRetired   KeyStatus = "retired"   // 已退休：不可再被新写入引用
	KeyRevoking  KeyStatus = "revoking"  // 吊销中：等待引用清空
	KeyRevoked   KeyStatus = "revoked"   // 已吊销：不可再被任何写入引用
)

// Valid 校验密钥状态是否合法。
func (s KeyStatus) Valid() bool {
	switch s {
	case KeyCandidate, KeyActive, KeyRetiring, KeyRetired, KeyRevoking, KeyRevoked:
		return true
	default:
		return false
	}
}

// RetiredOrRevoked 判断密钥是否处于不可引用状态。
func (s KeyStatus) RetiredOrRevoked() bool {
	return s == KeyRetired || s == KeyRevoked
}

// Key 表示层级中的一个密钥版本。
type Key struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      KeyKind   `json:"kind"`
	Status    KeyStatus `json:"status"`
	ParentID  string    `json:"parentId,omitempty"` // 根密钥为空
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// KeyRef 是引用边（授权边/封装边）使用的密钥摘要视图。
type KeyRef struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Kind   KeyKind   `json:"kind"`
	Status KeyStatus `json:"status"`
}
