package service

import (
	"context"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/plan"
)

// ---- 轮换计划 ----

// CreatePlan 创建轮换计划。
func (a *App) CreatePlan(ctx context.Context, name string) (*model.Plan, error) {
	return a.Plans.CreatePlan(ctx, NewID("plan"), name)
}

// ListPlans 列出全部计划。
func (a *App) ListPlans(ctx context.Context) ([]*model.Plan, error) {
	return a.Plans.ListPlans(ctx)
}

// GetPlan 查询计划。
func (a *App) GetPlan(ctx context.Context, id string) (*model.Plan, error) {
	return a.Plans.GetPlan(ctx, id)
}

// ListPlanSteps 列出计划步骤。
func (a *App) ListPlanSteps(ctx context.Context, planID string) ([]*model.PlanStep, error) {
	return a.Plans.ListSteps(ctx, planID)
}

// AddPlanStep 追加计划步骤。
func (a *App) AddPlanStep(ctx context.Context, planID string, action model.StepAction,
	keyID, objectID, subjectID, targetKeyID string) (*model.PlanStep, error) {
	step := &model.PlanStep{
		ID:          NewID("step"),
		Action:      action,
		KeyID:       keyID,
		ObjectID:    objectID,
		SubjectID:   subjectID,
		TargetKeyID: targetKeyID,
	}
	return a.Plans.AddStep(ctx, planID, step)
}

// ValidatePlan 验证计划全部步骤。
func (a *App) ValidatePlan(ctx context.Context, planID string) (*plan.ValidationReport, error) {
	return a.Plans.Validate(ctx, planID, a.Validator)
}

// ApprovePlan 批准计划执行。
func (a *App) ApprovePlan(ctx context.Context, planID string) (*plan.ApproveReport, error) {
	return a.Plans.Approve(ctx, planID, a.Engine)
}

// ExecutePlanStep 推进计划步骤。
func (a *App) ExecutePlanStep(ctx context.Context, planID string, seq int) (*executionResult, error) {
	res, err := a.Runner.ExecuteStep(ctx, planID, seq)
	if err != nil {
		return nil, err
	}
	return &executionResult{
		ExecutionID:  res.Execution.ID,
		StepID:       res.Execution.StepID,
		Action:       string(res.Execution.Action),
		Status:       string(res.Execution.Status),
		PlanStatus:   string(res.PlanStatus),
		AppliedCount: res.Applied,
		TotalSteps:   res.Total,
		Idempotent:   res.Idempotent,
	}, nil
}

// RollbackPlan 回滚最后已应用步骤。
func (a *App) RollbackPlan(ctx context.Context, planID string) (*rollbackResult, error) {
	res, err := a.Runner.RollbackLastStep(ctx, planID)
	if err != nil {
		return nil, err
	}
	return &rollbackResult{
		ExecutionID: res.Execution.ID,
		StepID:      res.StepID,
		Seq:         res.Seq,
		Action:      string(res.Action),
		PlanStatus:  string(res.PlanStatus),
	}, nil
}

// RecoverPlan 重启恢复检查。
func (a *App) RecoverPlan(ctx context.Context, planID string) (*recoveryReport, error) {
	rep, err := a.Runner.Recover(ctx, planID, a.Validator, a.Engine)
	if err != nil {
		return nil, err
	}
	return &recoveryReport{
		PlanID:         rep.PlanID,
		PlanStatus:     string(rep.PlanStatus),
		AppliedSteps:   rep.AppliedSteps,
		RemainingSteps: rep.RemainingSteps,
		ProofPreserved: rep.ProofPreserved,
		FingerprintOK:  rep.FingerprintOK,
		VerifiedCount:  rep.VerifiedCount,
		BlockedCount:   rep.BlockedCount,
		Messages:       rep.Messages,
	}, nil
}

// 供 HTTP 层返回的轻量结果视图。
type executionResult struct {
	ExecutionID  string `json:"executionId"`
	StepID       string `json:"stepId"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	PlanStatus   string `json:"planStatus"`
	AppliedCount int    `json:"appliedCount"`
	TotalSteps   int    `json:"totalSteps"`
	Idempotent   bool   `json:"idempotent"`
}

type rollbackResult struct {
	ExecutionID string `json:"executionId"`
	StepID      string `json:"stepId"`
	Seq         int    `json:"seq"`
	Action      string `json:"action"`
	PlanStatus  string `json:"planStatus"`
}

type recoveryReport struct {
	PlanID          string   `json:"planId"`
	PlanStatus      string   `json:"planStatus"`
	AppliedSteps    int      `json:"appliedSteps"`
	RemainingSteps  int      `json:"remainingSteps"`
	ProofPreserved  bool     `json:"proofPreserved"`
	FingerprintOK   bool     `json:"fingerprintOk"`
	VerifiedCount   int      `json:"verifiedCount"`
	BlockedCount    int      `json:"blockedCount"`
	Messages        []string `json:"messages"`
}
