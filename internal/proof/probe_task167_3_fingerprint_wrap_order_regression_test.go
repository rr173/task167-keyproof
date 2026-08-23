package proof

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

func TestBug03_FingerprintIgnoresEquivalentWrapInsertionOrder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := relation.NewRegistry(st)
	eng := NewEngine(st, reg)
	root := &model.Key{ID: "key-root", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.ActivateKey(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	first := &model.Key{ID: "key-first", Name: "first", Kind: model.KindData, ParentID: root.ID, Status: model.KeyCandidate}
	second := &model.Key{ID: "key-second", Name: "second", Kind: model.KindData, ParentID: root.ID, Status: model.KeyCandidate}
	for _, key := range []*model.Key{first, second} {
		if err := reg.CreateKey(ctx, key); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.ActivateKey(ctx, key.ID); err != nil {
			t.Fatal(err)
		}
	}
	obj := &model.Object{ID: "obj-payload", Name: "payload", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	for _, keyID := range []string{first.ID, second.ID} {
		if err := relation.WrapServiceOf(reg).AddWrap(ctx, obj.ID, keyID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE wraps SET created_at = '2026-01-01T00:00:00Z' WHERE object_id = ?`, obj.ID); err != nil {
		t.Fatal(err)
	}
	fp1, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM wraps WHERE object_id = ?`, obj.ID); err != nil {
		t.Fatal(err)
	}
	for _, keyID := range []string{second.ID, first.ID} {
		if _, err := st.DB().ExecContext(ctx, `INSERT INTO wraps(object_id,key_id,created_at) VALUES(?,?,?)`, obj.ID, keyID, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	fp2, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("equivalent wrap sets changed fingerprint: %s != %s", fp1, fp2)
	}
}
