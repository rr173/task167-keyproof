package plan_test

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

func TestBug02_RewrapValidationRequiresExistingSourceEdge(t *testing.T) {
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
	oldMissing, err := app.CreateKey(ctx, "old-missing", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, oldMissing.ID); err != nil {
		t.Fatal(err)
	}
	oldActual, err := app.CreateKey(ctx, "old-actual", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, oldActual.ID); err != nil {
		t.Fatal(err)
	}
	target, err := app.CreateKey(ctx, "target", model.KindData, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	owner, err := app.CreateSubject(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddGrant(ctx, owner.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	obj, err := app.CreateObject(ctx, "payload")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Wrap(ctx, obj.ID, oldActual.ID); err != nil {
		t.Fatal(err)
	}
	p, err := app.CreatePlan(ctx, "rewrap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, p.ID, model.ActionRewrapObject, oldMissing.ID, obj.ID, "", target.ID); err != nil {
		t.Fatal(err)
	}
	report, err := app.ValidatePlan(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != model.PlanBlocked || len(report.Blocked) != 1 {
		t.Fatalf("validation = %#v, want one blocked step for missing source edge", report)
	}
}
