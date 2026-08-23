package plan

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/proof"
)

// ApproveReport 批准执行前的最终一致性检查报告。
type ApproveReport struct {
	PlanID            string           `json:"planId"`
	Status            model.PlanStatus `json:"status"`
	FingerprintOK     bool             `json:"fingerprintOk"`
	RootAuthOK        bool             `json:"rootAuthOk"`
	ResidualFree      bool             `json:"residualFree"`
	Approved          bool             `json:"approved"`
	Messages          []string         `json:"messages"`
	ExpectedFingerprint string         `json:"expectedFingerprint,omitempty"`
}

// Approve 批准计划执行。前置条件：
//  1. 计划处于 executable 状态且全部步骤 verified；
//  2. 输入边指纹与最近证明快照一致（未被篡改）；
//  3. 根密钥均存在授权主体（根链有效）；
//  4. 无退休残留引用。
//
// 批准本身幂等：重复调用返回相同报告。
func (s *Service) Approve(ctx context.Context, planID string, engine *proof.Engine) (*ApproveReport, error) {
	p, err := s.plans.GetPlan(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	report := &ApproveReport{PlanID: planID, Status: p.Status}
	if p.Status != model.PlanExecutable {
		report.Messages = append(report.Messages,
			fmt.Sprintf("计划状态为 %s，仅 %s 可批准", p.Status, model.PlanExecutable))
		return report, nil
	}
	steps, err := s.plans.ListSteps(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.Status == model.StepPending {
			report.Messages = append(report.Messages,
				fmt.Sprintf("步骤 %d (%s) 未验证通过（状态 %s）", step.Seq, step.Action, step.Status))
			return report, nil
		}
	}

	// 指纹校验：与最近证明快照比对。
	recentProof, err := s.recentProof(ctx, planID, engine)
	if err != nil {
		return nil, err
	}
	if recentProof != nil {
		ok, err := engine.VerifyFingerprint(ctx, recentProof.InputFP)
		if err != nil {
			return nil, err
		}
		report.FingerprintOK = ok
		report.ExpectedFingerprint = recentProof.InputFP
		if !ok {
			report.Messages = append(report.Messages, "输入边指纹变化：证明快照已失效，请重新验证")
		}
	} else {
		report.FingerprintOK = true
	}

	// 根链授权检查。
	rootKeys, err := engine.RootKeysWithoutAuth(ctx)
	if err != nil {
		return nil, err
	}
	if len(rootKeys) == 0 {
		report.RootAuthOK = true
	} else {
		for _, k := range rootKeys {
			report.Messages = append(report.Messages,
				fmt.Sprintf("根密钥 %s 缺少授权主体", k.Name))
		}
	}

	// 退休残留检查。
	residuals, err := engine.ListResiduals(ctx)
	if err != nil {
		return nil, err
	}
	report.ResidualFree = len(residuals) == 0
	for _, r := range residuals {
		report.Messages = append(report.Messages, r.Summary)
	}

	report.Approved = report.FingerprintOK && report.RootAuthOK && report.ResidualFree
	return report, nil
}

// recentProof 返回计划最近的证明快照（按创建时间倒序）。
func (s *Service) recentProof(ctx context.Context, planID string, engine *proof.Engine) (*model.Proof, error) {
	proofs, err := engine.ListProofs(ctx, planID)
	if err != nil {
		return nil, err
	}
	if len(proofs) == 0 {
		return nil, nil
	}
	return proofs[len(proofs)-1], nil
}
