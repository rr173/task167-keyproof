package model

import "errors"

// 领域错误码。HTTP 层将错误码映射为 4xx/5xx 响应，
// 业务包通过 errors.Is 判断失败原因。
var (
	// ErrNotFound 资源不存在。
	ErrNotFound = errors.New("not_found")
	// ErrConflict 资源冲突（重复登记/重复边）。
	ErrConflict = errors.New("conflict")
	// ErrInvalidState 状态机不允许的迁移。
	ErrInvalidState = errors.New("invalid_state")
	// ErrInvalidInput 请求参数不合法。
	ErrInvalidInput = errors.New("invalid_input")
	// ErrCycleDetected 密钥层级存在环。
	ErrCycleDetected = errors.New("cycle_detected")
	// ErrNoRootAuth 根密钥缺少授权主体。
	ErrNoRootAuth = errors.New("root_key_without_authorization")
	// ErrRetiredReference 尝试为已退休/已吊销密钥建立新引用。
	ErrRetiredReference = errors.New("retired_key_reference")
	// ErrNoCoverage 对象不存在任何可解密路径。
	ErrNoCoverage = errors.New("no_coverage")
	// ErrRetirementBlocked 退休动作会令对象失去覆盖。
	ErrRetirementBlocked = errors.New("retirement_blocked")
	// ErrCrossPlan 目标被其它执行中的计划占用。
	ErrCrossPlan = errors.New("cross_plan_execution")
	// ErrImmutable 已完成证明引用的输入边不可改写。
	ErrImmutable = errors.New("immutable_proven_input")
	// ErrRollbackForbidden 回滚目标涉及已退休密钥，禁止回滚。
	ErrRollbackForbidden = errors.New("rollback_forbidden")
	// ErrPlanNotExecutable 计划不在可执行状态。
	ErrPlanNotExecutable = errors.New("plan_not_executable")
	// ErrNotDrafted 计划不在草拟状态，无法追加步骤。
	ErrNotDrafted = errors.New("plan_not_drafted")
	// ErrEmptyPlan 计划没有任何步骤。
	ErrEmptyPlan = errors.New("empty_plan")
)
