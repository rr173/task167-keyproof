package execution

import (
	"context"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// RollbackResult 回滚结果。
type RollbackResult struct {
	Execution *model.Execution `json:"execution"`
	StepID    string           `json:"stepId"`
	Seq       int              `json:"seq"`
	Action    model.StepAction `json:"action"`
	PlanStatus model.PlanStatus `json:"planStatus"`
}

// RollbackLastStep 回滚计划最后一个已应用步骤。
// 约束：回滚仅可作用于尚未退休的密钥——若步骤涉及的目标密钥
// 已进入 retired 状态，则禁止回滚。
// 回滚后计划回到 validating，需重新验证才能再次执行。
func (r *Runner) RollbackLastStep(ctx context.Context, planID string) (*RollbackResult, error) {
	p, err := r.plans.GetPlan(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.PlanExecutable && p.Status != model.PlanCompleted {
		return nil, model.ErrInvalidState
	}
	steps, err := r.plans.ListSteps(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	var last *model.PlanStep
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Status == model.StepApplied {
			last = steps[i]
			break
		}
	}
	if last == nil {
		return nil, model.ErrNotFound
	}

	// 受限回滚检查：目标密钥不得已退休。
	if err := r.checkRollbackAllowed(ctx, last); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	exec := &model.Execution{
		ID:        planID + "-" + last.ID + "-rb-" + fmt.Sprintf("%d", started.UnixNano()),
		PlanID:    planID,
		StepID:    last.ID,
		Action:    last.Action,
		Status:    model.ExecStarted,
		StartedAt: started,
	}
	applyErr := r.st.WithTx(func(tx *store.Tx) error {
		if err := r.undoStep(ctx, tx, last); err != nil {
			return err
		}
		if err := r.plans.UpdateStepStatus(ctx, tx, last.ID, model.StepRolledBack, nil); err != nil {
			return err
		}
		if err := r.execs.Insert(ctx, tx, exec); err != nil {
			return err
		}
		return nil
	})
	if applyErr != nil {
		exec.Status = model.ExecFailed
		exec.Reason = applyErr.Error()
		now := time.Now().UTC()
		exec.FinishedAt = &now
		_ = r.execs.Insert(ctx, r.st.DB(), exec)
		return nil, applyErr
	}
	now := time.Now().UTC()
	exec.FinishedAt = &now
	exec.Status = model.ExecRolledBack
	exec.Reason = "rolled back"
	if err := r.execs.Finish(ctx, r.st.DB(), exec.ID, model.ExecRolledBack, "rolled back"); err != nil {
		return nil, err
	}

	// 回滚后计划回到验证态；释放该步骤锁。
	_ = r.releaseStepLocks(ctx, planID, last)
	if err := r.plans.UpdatePlanStatus(ctx, r.st.DB(), planID, model.PlanValidating); err != nil {
		return nil, err
	}
	return &RollbackResult{
		Execution:  exec,
		StepID:     last.ID,
		Seq:        last.Seq,
		Action:     last.Action,
		PlanStatus: model.PlanValidating,
	}, nil
}

// checkRollbackAllowed 校验回滚目标涉及的密钥尚未退休。
func (r *Runner) checkRollbackAllowed(ctx context.Context, step *model.PlanStep) error {
	keyIDs := []string{}
	switch step.Action {
	case model.ActionRetireKey, model.ActionRevokeKey, model.ActionGrantKey, model.ActionRevokeGrant:
		keyIDs = append(keyIDs, step.KeyID)
	case model.ActionRewrapObject:
		keyIDs = append(keyIDs, step.KeyID, step.TargetKeyID)
	case model.ActionRemoveWrap:
		keyIDs = append(keyIDs, step.KeyID)
	}
	for _, id := range keyIDs {
		if id == "" {
			continue
		}
		k, err := r.keys.Get(ctx, r.st.DB(), id)
		if err != nil {
			return err
		}
		if k.Status == model.KeyRetired {
			return fmt.Errorf("%w: 密钥 %s 已退休", model.ErrRollbackForbidden, k.Name)
		}
	}
	return nil
}

// undoStep 在事务内执行步骤的逆操作。
func (r *Runner) undoStep(ctx context.Context, tx *store.Tx, step *model.PlanStep) error {
	switch step.Action {
	case model.ActionRetireKey:
		// retiring -> active。
		k, err := r.keys.Get(ctx, tx, step.KeyID)
		if err != nil {
			return err
		}
		if k.Status == model.KeyRetiring {
			return r.keys.UpdateStatus(ctx, tx, k.ID, model.KeyActive)
		}
		return model.ErrInvalidState
	case model.ActionRevokeKey:
		// revoked -> active（吊销尚未产生新引用的前提下可回滚）。
		k, err := r.keys.Get(ctx, tx, step.KeyID)
		if err != nil {
			return err
		}
		if k.Status == model.KeyRevoked {
			return r.keys.UpdateStatus(ctx, tx, k.ID, model.KeyActive)
		}
		return model.ErrInvalidState
	case model.ActionRewrapObject:
		// 反向重新封装：new -> old。
		return r.wraps.RewrapTx(ctx, tx, step.ObjectID, step.TargetKeyID, step.KeyID)
	case model.ActionGrantKey:
		return r.grants.RevokeGrantTx(ctx, tx, step.SubjectID, step.KeyID)
	case model.ActionRevokeGrant:
		return r.grants.AddGrantTx(ctx, tx, step.SubjectID, step.KeyID)
	case model.ActionRemoveWrap:
		return r.wraps.AddWrapTx(ctx, tx, step.ObjectID, step.KeyID)
	default:
		return model.ErrInvalidInput
	}
}
