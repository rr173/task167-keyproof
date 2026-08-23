package proof

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

func TestBug07_FingerprintIgnoresEquivalentGrantInsertionOrder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := relation.NewRegistry(st)
	eng := NewEngine(st, reg)
	key := &model.Key{ID: "root", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := reg.CreateSubject(ctx, &model.Subject{ID: id, Name: id, Status: model.SubjectActive}); err != nil {
			t.Fatal(err)
		}
		if err := relation.GrantServiceOf(reg).AddGrant(ctx, id, key.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE grants SET created_at = '2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	fp1, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM grants WHERE key_id = ?`, key.ID); err != nil {
		t.Fatal(err)
	}
	for _, subjectID := range []string{"b", "a"} {
		if _, err := st.DB().ExecContext(ctx, `INSERT INTO grants(subject_id,key_id,created_at) VALUES(?,?,?)`, subjectID, key.ID, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	fp2, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("equivalent grant sets changed fingerprint: %s != %s", fp1, fp2)
	}
}
