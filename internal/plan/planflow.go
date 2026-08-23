// Package plan 实现轮换计划状态机：创建、追加步骤、单步验证、
// 批准与阻断报告。验证只读关系图并写入步骤状态，不产生数据变更。
package plan

import (
	"context"
	"fmt"
	"time"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// Service 编排计划生命周期。
type Service struct {
	st    *store.Store
	plans *store.PlanRepo
}

// NewService 构造计划服务。
func NewService(st *store.Store) *Service {
	return &Service{st: st, plans: store.NewPlanRepo()}
}

// CreatePlan 创建草拟状态的轮换计划。
func (s *Service) CreatePlan(ctx context.Context, id, name string) (*model.Plan, error) {
	p := &model.Plan{ID: id, Name: name, Status: model.PlanDrafted}
	if err := s.plans.CreatePlan(ctx, s.st.DB(), p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetPlan 查询计划。
func (s *Service) GetPlan(ctx context.Context, id string) (*model.Plan, error) {
	return s.plans.GetPlan(ctx, s.st.DB(), id)
}

// ListPlans 列出全部计划。
func (s *Service) ListPlans(ctx context.Context) ([]*model.Plan, error) {
	return s.plans.ListPlans(ctx, s.st.DB())
}

// ListSteps 列出计划步骤。
func (s *Service) ListSteps(ctx context.Context, planID string) ([]*model.PlanStep, error) {
	return s.plans.ListSteps(ctx, s.st.DB(), planID)
}

// AddStep 追加步骤（仅草拟状态允许）。
func (s *Service) AddStep(ctx context.Context, planID string, step *model.PlanStep) (*model.PlanStep, error) {
	p, err := s.plans.GetPlan(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.PlanDrafted {
		return nil, model.ErrNotDrafted
	}
	if !step.Action.Valid() {
		return nil, model.ErrInvalidInput
	}
	switch step.Action {
	case model.ActionRetireKey, model.ActionRevokeKey:
		if step.KeyID == "" {
			return nil, model.ErrInvalidInput
		}
	case model.ActionRewrapObject:
		if step.ObjectID == "" || step.KeyID == "" || step.TargetKeyID == "" {
			return nil, model.ErrInvalidInput
		}
	case model.ActionGrantKey, model.ActionRevokeGrant:
		if step.SubjectID == "" || step.KeyID == "" {
			return nil, model.ErrInvalidInput
		}
	case model.ActionRemoveWrap:
		if step.ObjectID == "" || step.KeyID == "" {
			return nil, model.ErrInvalidInput
		}
	}
	seq, err := s.plans.NextSeq(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	step.PlanID = planID
	step.Seq = seq
	step.Status = model.StepPending
	step.CreatedAt = time.Now().UTC()
	if err := s.plans.AddStep(ctx, s.st.DB(), step); err != nil {
		return nil, err
	}
	return step, nil
}

// validateAll 验证计划全部步骤（drafted/blocked -> validating -> 结果）。
func (s *Service) Validate(ctx context.Context, planID string, checker StepChecker) (*ValidationReport, error) {
	p, err := s.plans.GetPlan(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if p.Status != model.PlanDrafted && p.Status != model.PlanBlocked {
		return nil, model.ErrInvalidState
	}
	steps, err := s.plans.ListSteps(ctx, s.st.DB(), planID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, model.ErrEmptyPlan
	}

	if err := s.plans.UpdatePlanStatus(ctx, s.st.DB(), planID, model.PlanValidating); err != nil {
		return nil, err
	}
	report := &ValidationReport{PlanID: planID}
	blocked := false
	for _, step := range steps {
		result := checker.Check(ctx, step, steps)
		if result.Err != nil {
			blocked = true
			bt := time.Now().UTC()
			if err := s.plans.UpdateStepStatus(ctx, s.st.DB(), step.ID, model.StepBlocked, &bt); err != nil {
				return nil, err
			}
			report.Blocked = append(report.Blocked, result)
		} else {
			if err := s.plans.UpdateStepStatus(ctx, s.st.DB(), step.ID, model.StepVerified, nil); err != nil {
				return nil, err
			}
			report.Verified = append(report.Verified, result)
		}
	}

	if blocked {
		if err := s.plans.UpdatePlanStatus(ctx, s.st.DB(), planID, model.PlanBlocked); err != nil {
			return nil, err
		}
		report.Status = model.PlanBlocked
	} else {
		if err := s.plans.UpdatePlanStatus(ctx, s.st.DB(), planID, model.PlanExecutable); err != nil {
			return nil, err
		}
		report.Status = model.PlanExecutable
	}
	return report, nil
}

// ValidationReport 是整计划的验证结果。
type ValidationReport struct {
	PlanID   string              `json:"planId"`
	Status   model.PlanStatus    `json:"status"`
	Verified []StepCheckResult   `json:"verified"`
	Blocked  []StepCheckResult   `json:"blocked"`
}

// StepCheckResult 单步验证结果。
type StepCheckResult struct {
	StepID    string          `json:"stepId"`
	Seq       int             `json:"seq"`
	Action    model.StepAction `json:"action"`
	Summary   string          `json:"summary"`
	Chain     *model.EvidenceChain `json:"chain,omitempty"`
	Err       error           `json:"-"`
	ErrCode   string          `json:"errorCode,omitempty"`
	ErrDetail string          `json:"errorDetail,omitempty"`
}

// StepChecker 由 plan 包的调用方（service 层）注入，以访问
// relation/proof 能力，避免 plan 包反向依赖业务图。
// planSteps 是计划的全部步骤（按序号升序），供验证器做
// “前序步骤已应用”的假设状态推演。
type StepChecker interface {
	Check(ctx context.Context, step *model.PlanStep, planSteps []*model.PlanStep) StepCheckResult
}

// ErrSummary 生成阻断步骤的总结。
func ErrSummary(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
