package execution_test

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

func TestExecuteStepIsIdempotentAfterApplication(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := service.New(st)

	root, err := app.CreateKey(ctx, "root", model.KindRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	owner, err := app.CreateSubject(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddGrant(ctx, owner.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	target, err := app.CreateSubject(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreatePlan(ctx, "grant target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionGrantKey, root.ID, "", target.ID, ""); err != nil {
		t.Fatal(err)
	}
	if report, err := app.ValidatePlan(ctx, plan.ID); err != nil || report.Status != model.PlanExecutable {
		t.Fatalf("validate report = %#v, err = %v", report, err)
	}
	if report, err := app.ApprovePlan(ctx, plan.ID); err != nil || !report.Approved {
		t.Fatalf("approve report = %#v, err = %v", report, err)
	}
	first, err := app.ExecutePlanStep(ctx, plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.ExecutePlanStep(ctx, plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Idempotent || second.ExecutionID != first.ExecutionID || second.AppliedCount != 1 {
		t.Fatalf("second execution = %#v; first = %#v", second, first)
	}
}

// TestBlockedPlanRecoversToExecutableAfterExternalFix 验证：
// 计划因覆盖不足被 blocked 后，外部关系修复后再次验证必须把计划与步骤
// 恢复到 executable/verified，并允许随后执行原步骤。
func TestBlockedPlanRecoversToExecutableAfterExternalFix(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := service.New(st)

	root, err := app.CreateKey(ctx, "root", model.KindRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	eng, err := app.CreateSubject(ctx, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddGrant(ctx, eng.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	dataA, err := app.CreateKey(ctx, "dataA", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, dataA.ID); err != nil {
		t.Fatal(err)
	}
	dataB, err := app.CreateKey(ctx, "dataB", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, dataB.ID); err != nil {
		t.Fatal(err)
	}
	obj, err := app.CreateObject(ctx, "obj")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Wrap(ctx, obj.ID, dataA.ID); err != nil {
		t.Fatal(err)
	}

	// 计划：退休 dataA —— 此时 obj 仍仅由 dataA 子树覆盖，应被阻断。
	plan, err := app.CreatePlan(ctx, "retire-dataA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionRetireKey, dataA.ID, "", "", ""); err != nil {
		t.Fatal(err)
	}

	blocked, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != model.PlanBlocked || len(blocked.Blocked) != 1 {
		t.Fatalf("首次验证: 期望 blocked/1, 实际 %s/%d", blocked.Status, len(blocked.Blocked))
	}
	stepsBefore, err := app.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stepsBefore[0].Status != model.StepBlocked {
		t.Fatalf("阻断步骤状态 = %s, 期望 blocked", stepsBefore[0].Status)
	}

	// 外部关系修复：把对象重新封装到子树外，退休不再使对象失去覆盖。
	if err := app.Rewrap(ctx, obj.ID, dataA.ID, dataB.ID); err != nil {
		t.Fatal(err)
	}

	// 再次验证：计划与步骤必须恢复为 executable/verified。
	revalidated, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("再次验证失败: %v", err)
	}
	if revalidated.Status != model.PlanExecutable {
		t.Fatalf("再次验证: 期望 executable, 实际 %s (verified=%d blocked=%d)",
			revalidated.Status, len(revalidated.Verified), len(revalidated.Blocked))
	}
	if len(revalidated.Verified) != 1 || len(revalidated.Blocked) != 0 {
		t.Fatalf("再次验证: 期望 verified=1/blocked=0, 实际 verified=%d blocked=%d",
			len(revalidated.Verified), len(revalidated.Blocked))
	}
	stepsAfter, err := app.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stepsAfter[0].Status != model.StepVerified {
		t.Fatalf("修复后步骤状态 = %s, 期望 verified", stepsAfter[0].Status)
	}
	if stepsAfter[0].BlockedAt != nil {
		t.Fatalf("已验证步骤不应携带 blockedAt: %v", stepsAfter[0].BlockedAt)
	}

	// 批准并执行原步骤必须成功。
	appr, err := app.ApprovePlan(ctx, plan.ID)
	if err != nil || !appr.Approved {
		t.Fatalf("批准: err=%v approved=%v msgs=%v", err, appr.Approved, appr.Messages)
	}
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err != nil {
		t.Fatalf("执行修复后的原步骤失败: %v", err)
	}
	keyA, _ := app.GetKey(ctx, dataA.ID)
	if keyA.Status != model.KeyRetiring {
		t.Fatalf("密钥A应进入退役待清理, 实际 %s", keyA.Status)
	}
}

// TestVerifiedStepDoesNotCarryBlockedAt 验证：通过验证的步骤不得残留
// blockedAt 时间戳（否则与“已验证”语义冲突）。
func TestVerifiedStepDoesNotCarryBlockedAt(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := service.New(st)

	root, err := app.CreateKey(ctx, "root", model.KindRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	owner, err := app.CreateSubject(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddGrant(ctx, owner.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreatePlan(ctx, "grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionGrantKey, root.ID, "", owner.ID, ""); err != nil {
		t.Fatal(err)
	}
	rep, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil || rep.Status != model.PlanExecutable {
		t.Fatalf("验证: err=%v status=%s", err, rep.Status)
	}
	steps, err := app.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].Status != model.StepVerified {
		t.Fatalf("步骤状态 = %s, 期望 verified", steps[0].Status)
	}
	if steps[0].BlockedAt != nil {
		t.Fatalf("已验证步骤不应携带 blockedAt: %v", steps[0].BlockedAt)
	}
}
