package proof

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

func TestBug01_RemovedSubjectCannotCoverObject(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := relation.NewRegistry(st)
	eng := NewEngine(st, reg)
	key := &model.Key{ID: "key-root", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.ActivateKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	sub := &model.Subject{ID: "sub-removed", Name: "former", Status: model.SubjectActive}
	if err := reg.CreateSubject(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if err := relation.GrantServiceOf(reg).AddGrant(ctx, sub.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	obj := &model.Object{ID: "obj-secret", Name: "secret", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	if err := relation.WrapServiceOf(reg).AddWrap(ctx, obj.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE subjects SET status = 'removed' WHERE id = ?`, sub.ID); err != nil {
		t.Fatal(err)
	}
	paths, err := eng.CoverageOfObject(ctx, obj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("removed subject still covers object: %#v", paths)
	}
	chain, err := eng.ShortestPath(ctx, sub.ID, obj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chain == nil || chain.Kind != "gap" {
		t.Fatalf("removed subject path = %#v, want gap", chain)
	}
}
