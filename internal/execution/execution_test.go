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

// TestRecoveryRecognizesRewrapped 验证对象重新封装成功后状态进入 rewrapped，
// 重启恢复流程能据此识别该对象已完成重新封装。
func TestRecoveryRecognizesRewrapped(t *testing.T) {
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
	obj, err := app.CreateObject(ctx, "卷")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Wrap(ctx, obj.ID, dataA.ID); err != nil {
		t.Fatal(err)
	}

	plan, err := app.CreatePlan(ctx, "重封装")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionRewrapObject,
		dataA.ID, obj.ID, "", dataB.ID); err != nil {
		t.Fatal(err)
	}
	if report, err := app.ValidatePlan(ctx, plan.ID); err != nil || report.Status != model.PlanExecutable {
		t.Fatalf("validate report = %#v, err = %v", report, err)
	}
	if report, err := app.ApprovePlan(ctx, plan.ID); err != nil || !report.Approved {
		t.Fatalf("approve report = %#v, err = %v", report, err)
	}
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err != nil {
		t.Fatalf("execute rewrap: %v", err)
	}

	// 执行重封装后对象状态必须为 rewrapped。
	got, err := app.GetObject(ctx, obj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Status.Rewrapped() {
		t.Fatalf("对象状态应为 rewrapped, 实际 %s", got.Status)
	}

	// 恢复流程据此识别已完成重封装的对象。
	rec, err := app.RecoverPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.RewrappedCount != 1 {
		t.Fatalf("恢复应识别 1 个已完成重封装对象, 实际 %d", rec.RewrappedCount)
	}
	if !rec.ProofPreserved {
		t.Fatalf("证明应保持: %+v", rec)
	}
}
