package execution

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/plan"
	"task167-keyproof/internal/proof"
)

// RecoveryReport 是重启恢复检查报告。
type RecoveryReport struct {
	PlanID          string           `json:"planId"`
	PlanStatus      model.PlanStatus `json:"planStatus"`
	AppliedSteps    int              `json:"appliedSteps"`
	RemainingSteps  int              `json:"remainingSteps"`
	ProofPreserved  bool             `json:"proofPreserved"`  // 已完成步骤证明未被改写
	FingerprintOK   bool             `json:"fingerprintOk"`   // 当前指纹与最新证明一致
	VerifiedCount   int              `json:"verifiedCount"`   // 本次恢复重新验证通过的步骤数
	BlockedCount    int              `json:"blockedCount"`    // 本次恢复被阻断的步骤数
	RewrappedCount  int              `json:"rewrappedCount"`  // 已完成重新封装的对象数
	Messages        []string         `json:"messages"`
}

// Recover 重启恢复：从计划的最后已应用步骤继续验证剩余步骤。
// 已完成步骤的输入边与证明哈希保持原样；重复调用不重复计数。
func (r *Runner) Recover(ctx context.Context, planID string, validator *plan.Validator, engine *proof.Engine) (*RecoveryReport, error) {
	p, err := r.plans.GetPlan(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	steps, err := r.plans.ListSteps(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	applied, err := r.plans.AppliedCount(ctx, r.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	report := &RecoveryReport{
		PlanID:         planID,
		PlanStatus:     p.Status,
		AppliedSteps:   applied,
		RemainingSteps: len(steps) - applied,
	}

	// 1. 已完成步骤的证明保持校验：对每个 applied 步骤，
	//    其证明快照的输入指纹必须与当前输入一致。
	preserved := true
	for _, s := range steps {
		if s.Status != model.StepApplied {
			continue
		}
		proofSnap, err := engine.GetProofByStep(ctx, planID, s.ID)
		if err != nil {
			return nil, err
		}
		if proofSnap != nil {
			ok, err := engine.VerifyFingerprint(ctx, proofSnap.InputFP)
			if err != nil {
				return nil, err
			}
			if !ok {
				preserved = false
				report.Messages = append(report.Messages,
					fmt.Sprintf("步骤 %d (%s) 的输入边已被改写，证明失效", s.Seq, s.Action))
			}
		}
	}
	report.ProofPreserved = preserved

	// 2. 识别已完成重新封装的对象：每个已应用的 rewrap 步骤，
	//    其目标对象状态必须为 rewrapped——据此确认重封装已成功落地。
	rewrappedSeen := map[string]bool{}
	for _, s := range steps {
		if s.Status != model.StepApplied || s.Action != model.ActionRewrapObject {
			continue
		}
		obj, err := engine.ObjectOf(ctx, s.ObjectID)
		if err != nil {
			return nil, err
		}
		if obj.Status.Rewrapped() {
			rewrappedSeen[obj.ID] = true
			continue
		}
		preserved = false
		report.Messages = append(report.Messages,
			fmt.Sprintf("步骤 %d 的对象 %s 未进入 rewrapped（实际 %s），重封装未完成",
				s.Seq, obj.ID, obj.Status))
	}
	report.RewrappedCount = len(rewrappedSeen)
	report.ProofPreserved = preserved

	// 3. 对未应用且未回滚的步骤重新验证。
	for _, s := range steps {
		if s.Status == model.StepApplied || s.Status == model.StepRolledBack {
			continue
		}
		res := validator.Check(ctx, s, steps)
		if res.Err != nil {
			report.BlockedCount++
			if err := r.plans.UpdateStepStatus(ctx, r.st.DB(), s.ID, model.StepBlocked, nil); err != nil {
				return nil, err
			}
			report.Messages = append(report.Messages, fmt.Sprintf("步骤 %d 被阻断: %s", s.Seq, res.ErrDetail))
		} else {
			report.VerifiedCount++
			if err := r.plans.UpdateStepStatus(ctx, r.st.DB(), s.ID, model.StepVerified, nil); err != nil {
				return nil, err
			}
		}
	}

	// 4. 汇总：若全部步骤已应用则计划已完成；否则按阻断情况置状态。
	switch {
	case applied == len(steps):
		report.PlanStatus = model.PlanCompleted
	case report.BlockedCount > 0:
		report.PlanStatus = model.PlanBlocked
	case report.VerifiedCount > 0 || p.Status == model.PlanExecutable:
		report.PlanStatus = model.PlanExecutable
	default:
		report.PlanStatus = p.Status
	}
	if report.PlanStatus != p.Status {
		if err := r.plans.UpdatePlanStatus(ctx, r.st.DB(), planID, report.PlanStatus); err != nil {
			return nil, err
		}
	}
	return report, nil
}
