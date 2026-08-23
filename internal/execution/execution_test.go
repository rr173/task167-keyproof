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
