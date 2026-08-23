package relation

import (
	"context"
	"errors"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

func TestBug04_RetiringKeyCannotReceiveNewWrap(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := NewRegistry(st)
	key := &model.Key{ID: "key-retiring", Name: "retiring", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.ActivateKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.MarkRetiring(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	obj := &model.Object{ID: "obj-new", Name: "new", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	err = WrapServiceOf(reg).AddWrap(ctx, obj.ID, key.ID)
	if !errors.Is(err, model.ErrInvalidState) && !errors.Is(err, model.ErrRetiredReference) {
		t.Fatalf("retiring key wrap error = %v, want lifecycle rejection", err)
	}
}
