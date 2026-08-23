// Package execution 负责轮换计划的步骤推进、受限回滚与重启恢复。
// 推进遵循“同一计划一次只能推进一个步骤”的约束，并通过跨计划锁
// 阻止不同计划交叉执行同一目标。
package execution

import (
	"context"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

// Runner 执行计划步骤。
type Runner struct {
	st      *store.Store
	plans   *store.PlanRepo
	execs   *store.ExecutionRepo
	keys    *store.KeyRepo
	objects *store.ObjectRepo
	wraps   *relation.WrapService
	grants  *relation.GrantService
}

// NewRunner 构造执行器。
func NewRunner(st *store.Store, reg *relation.Registry) *Runner {
	return &Runner{
		st:      st,
		plans:   store.NewPlanRepo(),
		execs:   store.NewExecutionRepo(),
		keys:    store.NewKeyRepo(),
		objects: store.NewObjectRepo(),
		wraps:   relation.WrapServiceOf(reg),
		grants:  relation.GrantServiceOf(reg),
	}
}

// ListByPlan 列出计划的执行记录。
func (r *Runner) ListByPlan(ctx context.Context, planID string) ([]*model.Execution, error) {
	return r.execs.ListByPlan(ctx, r.st.DB(), planID)
}

// ListAll 列出全部执行记录。
func (r *Runner) ListAll(ctx context.Context) ([]*model.Execution, error) {
	return r.execs.ListAll(ctx, r.st.DB())
}

// StepResult 是执行步骤的结果。
type StepResult struct {
	Execution *model.Execution `json:"execution"`
	PlanStatus model.PlanStatus `json:"planStatus"`
	Applied   int              `json:"appliedCount"`
	Total     int              `json:"totalSteps"`
	Idempotent bool            `json:"idempotent"`
}

// ExecuteStep 推进计划的第 seq 个步骤。
// 幂等：若该步骤已应用，直接返回既有的 applied 记录，不重复计数。
func (r *Runner) ExecuteStep(ctx context.Context, planID string, seq int) (*StepResult, error) {
	p, err := r.plans.GetPlan(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.PlanExecutable && p.Status != model.PlanCompleted {
		return nil, model.ErrPlanNotExecutable
	}
	steps, err := r.plans.ListSteps(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if seq < 1 || seq > len(steps) {
		return nil, model.ErrNotFound
	}
	step := steps[seq-1]

	// 幂等：已应用直接返回既有执行记录。
	if step.Status == model.StepApplied {
		last, err := r.execs.LastExecution(ctx, r.st.DB(), planID)
		if err != nil {
			return nil, err
		}
		applied, _ := r.plans.AppliedCount(ctx, r.st.DB(), planID)
		return &StepResult{
			Execution:  last,
			PlanStatus: p.Status,
			Applied:    applied,
			Total:      len(steps),
			Idempotent: true,
		}, nil
	}
	if step.Status == model.StepBlocked {
		return nil, model.ErrInvalidState
	}

	// 目标锁定，阻止跨计划交叉执行。
	if err := r.acquireStepLocks(ctx, p.ID, step); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	exec := &model.Execution{
		ID:        planID + "-" + step.ID + "-" + fmt.Sprintf("%d", started.UnixNano()),
		PlanID:    planID,
		StepID:    step.ID,
		Action:    step.Action,
		Status:    model.ExecStarted,
		StartedAt: started,
	}

	// 应用变更（事务内）。
	applyErr := r.st.WithTx(func(tx *store.Tx) error {
		if err := r.applyStep(ctx, tx, step); err != nil {
			return err
		}
		if err := r.plans.UpdateStepStatus(ctx, tx, step.ID, model.StepApplied, nil); err != nil {
			return err
		}
		if err := r.execs.Insert(ctx, tx, exec); err != nil {
			return err
		}
		return nil
	})
	if applyErr != nil {
		// 失败：先释放本次获取的目标锁。锁在事务外申请，事务回滚不会回收，
		// 若不显式释放会遗留在失败路径，导致修复外部状态后重试同一步骤时
		// 被误判为跨计划占用（ErrCrossPlan）；再写入失败执行记录。
		_ = r.releaseStepLocks(ctx, p.ID, step)
		exec.Status = model.ExecFailed
		exec.Reason = applyErr.Error()
		now := time.Now().UTC()
		exec.FinishedAt = &now
		_ = r.execs.Insert(ctx, r.st.DB(), exec)
		return nil, applyErr
	}

	// 记录终态。
	now := time.Now().UTC()
	exec.FinishedAt = &now
	exec.Status = model.ExecApplied
	exec.Reason = "ok"
	if err := r.execs.Finish(ctx, r.st.DB(), exec.ID, model.ExecApplied, "ok"); err != nil {
		return nil, err
	}

	// 检查是否全部步骤已应用。
	applied, err := r.plans.AppliedCount(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	status := p.Status
	if applied == len(steps) {
		if err := r.plans.UpdatePlanStatus(ctx, r.st.DB(), planID, model.PlanCompleted); err != nil {
			return nil, err
		}
		status = model.PlanCompleted
		_ = r.execs.ReleasePlanLocks(ctx, r.st.DB(), planID)
	} else {
		_ = r.releaseStepLocks(ctx, p.ID, step)
	}
	return &StepResult{
		Execution:  exec,
		PlanStatus: status,
		Applied:    applied,
		Total:      len(steps),
		Idempotent: false,
	}, nil
}

// applyStep 在事务内应用步骤动作。
func (r *Runner) applyStep(ctx context.Context, tx *store.Tx, step *model.PlanStep) error {
	switch step.Action {
	case model.ActionRetireKey:
		k, err := r.keys.Get(ctx, tx, step.KeyID)
		if err != nil {
			return err
		}
		if k.Status == model.KeyActive {
			return r.keys.UpdateStatus(ctx, tx, k.ID, model.KeyRetiring)
		}
		if k.Status == model.KeyRetiring {
			return nil // 幂等
		}
		return model.ErrInvalidState
	case model.ActionRevokeKey:
		k, err := r.keys.Get(ctx, tx, step.KeyID)
		if err != nil {
			return err
		}
		if k.Status == model.KeyActive || k.Status == model.KeyRetiring {
			return r.keys.UpdateStatus(ctx, tx, k.ID, model.KeyRevoked)
		}
		return model.ErrInvalidState
	case model.ActionRewrapObject:
		return r.wraps.RewrapTx(ctx, tx, step.ObjectID, step.KeyID, step.TargetKeyID)
	case model.ActionGrantKey:
		return r.grants.AddGrantTx(ctx, tx, step.SubjectID, step.KeyID)
	case model.ActionRevokeGrant:
		return r.grants.RevokeGrantTx(ctx, tx, step.SubjectID, step.KeyID)
	case model.ActionRemoveWrap:
		return r.wraps.RemoveWrapTx(ctx, tx, step.ObjectID, step.KeyID)
	default:
		return model.ErrInvalidInput
	}
}

// acquireStepLocks 为步骤涉及的目标获取跨计划锁。
func (r *Runner) acquireStepLocks(ctx context.Context, planID string, step *model.PlanStep) error {
	targets := stepTargets(step)
	for _, t := range targets {
		if err := r.execs.AcquireLock(ctx, r.st.DB(), planID, t.typ, t.id); err != nil {
			// 回滚已获取的锁。
			for _, got := range targets {
				_ = r.execs.ReleaseLock(ctx, r.st.DB(), planID, got.typ, got.id)
				if got.typ == t.typ && got.id == t.id {
					break
				}
			}
			return err
		}
	}
	return nil
}

func (r *Runner) releaseStepLocks(ctx context.Context, planID string, step *model.PlanStep) error {
	for _, t := range stepTargets(step) {
		if err := r.execs.ReleaseLock(ctx, r.st.DB(), planID, t.typ, t.id); err != nil {
			return err
		}
	}
	return nil
}

type target struct{ typ, id string }

// stepTargets 返回步骤锁定的目标集合。
func stepTargets(step *model.PlanStep) []target {
	var out []target
	switch step.Action {
	case model.ActionRetireKey, model.ActionRevokeKey, model.ActionGrantKey, model.ActionRevokeGrant:
		if step.KeyID != "" {
			out = append(out, target{"key", step.KeyID})
		}
	case model.ActionRewrapObject, model.ActionRemoveWrap:
		if step.ObjectID != "" {
			out = append(out, target{"object", step.ObjectID})
		}
		if step.TargetKeyID != "" {
			out = append(out, target{"key", step.TargetKeyID})
		}
	}
	return out
}
