package plan_test

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

func TestBug10_BlockedPlanRevalidationRestoresExecutableSteps(t *testing.T) {
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
	old, err := app.CreateKey(ctx, "old", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := app.CreateKey(ctx, "target", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []*model.Key{old, target} {
		if _, err := app.Reg.ActivateKey(ctx, key.ID); err != nil {
			t.Fatal(err)
		}
	}
	obj, err := app.CreateObject(ctx, "payload")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Wrap(ctx, obj.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := app.CreatePlan(ctx, "repair blocked removal")
	if err != nil {
		t.Fatal(err)
	}
	step, err := app.AddPlanStep(ctx, plan.ID, model.ActionRemoveWrap, old.ID, obj.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil || first.Status != model.PlanBlocked || len(first.Blocked) != 1 {
		t.Fatalf("initial validation = %#v, err = %v", first, err)
	}
	steps, err := app.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Status != model.StepBlocked {
		t.Fatalf("initial step state = %#v", steps)
	}
	if err := app.AddGrant(ctx, owner.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.Wrap(ctx, obj.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	second, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != model.PlanExecutable {
		t.Fatalf("revalidation status = %s, want executable", second.Status)
	}
	steps, err = app.ListPlanSteps(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Status != model.StepVerified || steps[0].BlockedAt != nil {
		t.Fatalf("revalidated step state = %#v", steps)
	}
	approved, err := app.ApprovePlan(ctx, plan.ID)
	if err != nil || !approved.Approved {
		t.Fatalf("approve after repair = %#v, err = %v", approved, err)
	}
	if _, err := app.ExecutePlanStep(ctx, plan.ID, step.Seq); err != nil {
		t.Fatalf("repaired step did not execute: %v", err)
	}
}
