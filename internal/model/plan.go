package model

import "time"

// PlanStatus 轮换计划状态机：
//
//	drafted -> validating -> blocked | executable -> completed
//	blocked -> validating（修复后重新验证）
type PlanStatus string

const (
	PlanDrafted    PlanStatus = "drafted"    // 草拟：可追加步骤
	PlanValidating PlanStatus = "validating" // 验证中：正在执行单步验证
	PlanBlocked    PlanStatus = "blocked"    // 被阻断：存在覆盖缺口或退休阻断
	PlanExecutable PlanStatus = "executable" // 可执行：全部步骤通过验证并批准
	PlanCompleted  PlanStatus = "completed"  // 已完成：所有步骤已应用
)

// Valid 校验计划状态是否合法。
func (s PlanStatus) Valid() bool {
	switch s {
	case PlanDrafted, PlanValidating, PlanBlocked, PlanExecutable, PlanCompleted:
		return true
	default:
		return false
	}
}

// StepAction 计划步骤的动作类型。
type StepAction string

const (
	ActionRetireKey    StepAction = "retire_key"     // 退休密钥
	ActionRewrapObject StepAction = "rewrap_object"  // 重新封装对象到新密钥
	ActionGrantKey     StepAction = "grant_key"      // 为密钥新增授权主体
	ActionRevokeGrant  StepAction = "revoke_grant"   // 撤销授权边
	ActionRevokeKey    StepAction = "revoke_key"     // 吊销密钥
	ActionRemoveWrap   StepAction = "remove_wrap"    // 移除对象封装边
)

// Valid 校验步骤动作是否合法。
func (a StepAction) Valid() bool {
	switch a {
	case ActionRetireKey, ActionRewrapObject, ActionGrantKey,
		ActionRevokeGrant, ActionRevokeKey, ActionRemoveWrap:
		return true
	default:
		return false
	}
}

// StepStatus 步骤执行状态。
type StepStatus string

const (
	StepPending     StepStatus = "pending"      // 待验证
	StepVerified    StepStatus = "verified"     // 已验证（不产生缺口/阻断）
	StepApplied     StepStatus = "applied"      // 已应用
	StepRolledBack  StepStatus = "rolled_back"  // 已回滚
	StepBlocked     StepStatus = "blocked"      // 被验证阻断
)

// Valid 校验步骤状态是否合法。
func (s StepStatus) Valid() bool {
	switch s {
	case StepPending, StepVerified, StepApplied, StepRolledBack, StepBlocked:
		return true
	default:
		return false
	}
}

// PlanStep 是计划中的一个轮换步骤。
type PlanStep struct {
	ID        string       `json:"id"`
	PlanID    string       `json:"planId"`
	Seq       int          `json:"seq"` // 步骤序号，从 1 开始
	Action    StepAction   `json:"action"`
	KeyID     string       `json:"keyId,omitempty"`
	ObjectID  string       `json:"objectId,omitempty"`
	SubjectID string       `json:"subjectId,omitempty"`
	TargetKeyID string     `json:"targetKeyId,omitempty"` // 重新封装/授权目标密钥
	Status    StepStatus   `json:"status"`
	BlockedAt *time.Time   `json:"blockedAt,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
}

// Plan 表示一次分批轮换计划。
type Plan struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    PlanStatus `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
