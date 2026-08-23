package relation

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

func TestBug08_RewrapMarksObjectRewrapped(t *testing.T) {
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
	old := &model.Key{ID: "old", Name: "old", Kind: model.KindData, ParentID: root.ID, Status: model.KeyCandidate}
	target := &model.Key{ID: "target", Name: "target", Kind: model.KindData, ParentID: root.ID, Status: model.KeyCandidate}
	for _, key := range []*model.Key{old, target} {
		if err := reg.CreateKey(ctx, key); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.ActivateKey(ctx, key.ID); err != nil {
			t.Fatal(err)
		}
	}
	obj := &model.Object{ID: "obj", Name: "payload", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	ws := WrapServiceOf(reg)
	if err := ws.AddWrap(ctx, obj.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := ws.Rewrap(ctx, obj.ID, old.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.NewObjectRepo().Get(ctx, st.DB(), obj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.ObjectRewrapped {
		t.Fatalf("object status = %s, want rewrapped", got.Status)
	}
}
