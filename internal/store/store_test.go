package store

import (
	"context"
	"path/filepath"
	"testing"

	"task167-keyproof/internal/model"
)

func TestStorePersistsKeyAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "keyproof.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	keys := NewKeyRepo()
	want := &model.Key{ID: "key-persist", Name: "root", Kind: model.KindRoot, Status: model.KeyActive}
	if err := keys.Create(ctx, st.DB(), want); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := keys.Get(ctx, reopened.DB(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.Kind != want.Kind || got.Status != want.Status {
		t.Fatalf("reopened key = %#v, want %#v", got, want)
	}
}
