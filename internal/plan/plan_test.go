package plan

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

func TestAddStepAssignsMonotonicSequence(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := NewService(st)
	p, err := svc.CreatePlan(ctx, "plan-seq", "rotation")
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.AddStep(ctx, p.ID, &model.PlanStep{ID: "step-1", Action: model.ActionGrantKey, KeyID: "key-1", SubjectID: "subject-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.AddStep(ctx, p.ID, &model.PlanStep{ID: "step-2", Action: model.ActionRevokeGrant, KeyID: "key-1", SubjectID: "subject-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("step sequence = %d, %d; want 1, 2", first.Seq, second.Seq)
	}
	steps, err := svc.ListSteps(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[1].Status != model.StepPending {
		t.Fatalf("persisted steps = %#v", steps)
	}
}
