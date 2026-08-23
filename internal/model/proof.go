package model

import "time"

// ProofStatus 证明快照状态。
type ProofStatus string

const (
	ProofPending   ProofStatus = "pending"    // 待计算
	ProofSufficient ProofStatus = "sufficient" // 覆盖充分且无退休残留
	ProofGap       ProofStatus = "gap"        // 覆盖缺失：存在孤立对象
	ProofResidual  ProofStatus = "residual"   // 退休残留：已退休密钥仍被引用
)

// Valid 校验证明状态是否合法。
func (s ProofStatus) Valid() bool {
	switch s {
	case ProofPending, ProofSufficient, ProofGap, ProofResidual:
		return true
	default:
		return false
	}
}

// Proof 是某计划某步骤计算出的覆盖证明快照。
// 快照对输入边（授权边/封装边/密钥状态）取 SHA-256 指纹，
// 已完成步骤的输入边不可改写，指纹用于防篡改校验。
type Proof struct {
	ID        string      `json:"id"`
	PlanID    string      `json:"planId"`
	StepID    string      `json:"stepId"`
	Status    ProofStatus `json:"status"`
	Digest    string      `json:"digest"`     // 证明摘要
	InputFP   string      `json:"inputFp"`    // 输入边指纹
	Summary   string      `json:"summary"`    // 人类可读结论
	CreatedAt time.Time   `json:"createdAt"`
}

// ChainNode 证据链节点：覆盖路径或缺口/残留证据的路径元素。
type ChainNode struct {
	Role string `json:"role"` // subject|grant|key|ancestor|wrap|object|gap|retired
	Name string `json:"name"`
	ID   string `json:"id"`
}

// EvidenceChain 最短证据链：覆盖路径（主体可达对象）或
// 缺口链（对象到无授权祖先）或退休残留链（对象到退休密钥）。
type EvidenceChain struct {
	Kind    string      `json:"kind"`    // coverage|gap|residual
	Length  int         `json:"length"`  // 链长度（边数）
	Nodes   []ChainNode `json:"nodes"`   // 路径节点
	Summary string      `json:"summary"` // 结论
}

// ExecStatus 执行记录状态。
type ExecStatus string

const (
	ExecStarted   ExecStatus = "started"     // 开始
	ExecApplied   ExecStatus = "applied"     // 已应用
	ExecRolledBack ExecStatus = "rolled_back" // 已回滚
	ExecFailed    ExecStatus = "failed"      // 失败
)

// Valid 校验执行记录状态是否合法。
func (s ExecStatus) Valid() bool {
	switch s {
	case ExecStarted, ExecApplied, ExecRolledBack, ExecFailed:
		return true
	default:
		return false
	}
}

// Execution 是一次步骤执行记录。
type Execution struct {
	ID         string     `json:"id"`
	PlanID     string     `json:"planId"`
	StepID     string     `json:"stepId"`
	Action     StepAction `json:"action"`
	Status     ExecStatus `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// PlanLock 是跨计划交叉执行锁：同一目标对象/密钥同一时刻
// 只能被一个执行中的计划占用。
type PlanLock struct {
	PlanID     string `json:"planId"`
	TargetType string `json:"targetType"` // key|object
	TargetID   string `json:"targetId"`
}
