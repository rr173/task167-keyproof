package relation

import (
	"context"
	"errors"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

func TestBug05_MoveKeyRejectsIndirectCycle(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := NewRegistry(st)
	root := &model.Key{ID: "root", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatal(err)
	}
	a := &model.Key{ID: "a", Name: "a", Kind: model.KindTenant, ParentID: root.ID, Status: model.KeyCandidate}
	b := &model.Key{ID: "b", Name: "b", Kind: model.KindTenant, ParentID: root.ID, Status: model.KeyCandidate}
	for _, key := range []*model.Key{a, b} {
		if err := reg.CreateKey(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.MoveKey(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := reg.MoveKey(ctx, b.ID, a.ID); !errors.Is(err, model.ErrCycleDetected) {
		t.Fatalf("indirect cycle move error = %v, want cycle_detected", err)
	}
}
