package execution_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

func TestBug09_FailedExecutionReleasesLocksForRetry(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "keyproof.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
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
	plan, err := app.CreatePlan(ctx, "retry failed rewrap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionRewrapObject, old.ID, obj.ID, "", target.ID); err != nil {
		t.Fatal(err)
	}
	if report, err := app.ValidatePlan(ctx, plan.ID); err != nil || report.Status != model.PlanExecutable {
		t.Fatalf("validate report = %#v, err = %v", report, err)
	}
	if report, err := app.ApprovePlan(ctx, plan.ID); err != nil || !report.Approved {
		t.Fatalf("approve report = %#v, err = %v", report, err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE keys SET status = ? WHERE id = ?`, string(model.KeyRevoked), target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err == nil {
		t.Fatal("first execution unexpectedly succeeded")
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE keys SET status = ? WHERE id = ?`, string(model.KeyActive), target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err != nil {
		if errors.Is(err, model.ErrCrossPlan) {
			t.Fatalf("retry hit stale execution lock: %v", err)
		}
		t.Fatalf("retry failed: %v", err)
	}
	_ = st.Close()
	_ = os.Remove(dbPath)
}
