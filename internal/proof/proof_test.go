package proof

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

func setupProof(t *testing.T) (*store.Store, *Engine, context.Context) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	reg := relation.NewRegistry(st)
	return st, NewEngine(st, reg), context.Background()
}

func TestCoverageAndGap(t *testing.T) {
	st, eng, ctx := setupProof(t)
	reg := relation.NewRegistry(st)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	tenant := &model.Key{ID: "t1", Name: "tenant", Kind: model.KindTenant, Status: model.KeyCandidate, ParentID: "r1"}
	_ = reg.CreateKey(ctx, tenant)
	_, _ = reg.ActivateKey(ctx, "t1")
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "t1"}
	_ = reg.CreateKey(ctx, data)
	_, _ = reg.ActivateKey(ctx, "d1")

	eng1 := &model.Subject{ID: "s1", Name: "eng", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, eng1)
	tenantB := &model.Subject{ID: "s2", Name: "tenantB", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, tenantB)

	gs := relation.GrantServiceOf(reg)
	if err := gs.AddGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	ws := relation.WrapServiceOf(reg)
	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)
	if err := ws.AddWrap(ctx, "o1", "d1"); err != nil {
		t.Fatalf("wrap: %v", err)
	}

	// eng1 经根密钥授权可解密 o1（覆盖链）。
	chain, err := eng.ShortestPath(ctx, "s1", "o1")
	if err != nil {
		t.Fatalf("shortest path: %v", err)
	}
	if chain == nil || chain.Kind != "coverage" {
		t.Fatalf("期望覆盖链, 得到 %+v", chain)
	}
	// tenantB 无授权：缺口链。
	chain2, err := eng.ShortestPath(ctx, "s2", "o1")
	if err != nil {
		t.Fatalf("shortest path: %v", err)
	}
	if chain2 == nil || chain2.Kind != "gap" {
		t.Fatalf("期望缺口链, 得到 %+v", chain2)
	}
	// o1 非孤立（eng1 可解密）。
	orphaned, err := eng.IsOrphaned(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if orphaned {
		t.Fatal("o1 不应孤立")
	}
}

func TestResidualDetection(t *testing.T) {
	st, eng, ctx := setupProof(t)
	reg := relation.NewRegistry(st)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	sub := &model.Subject{ID: "s1", Name: "s", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, sub)
	obj := &model.Object{ID: "o1", Name: "o", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)
	gs := relation.GrantServiceOf(reg)
	_ = gs.AddGrant(ctx, "s1", "r1")
	ws := relation.WrapServiceOf(reg)
	_ = ws.AddWrap(ctx, "o1", "r1")
	// 退休前应有残留。
	_, _ = reg.MarkRetiring(ctx, "r1")
	_, _ = reg.RetireKey(ctx, "r1")
	residuals, err := eng.ListResiduals(ctx)
	if err != nil {
		t.Fatalf("residuals: %v", err)
	}
	if len(residuals) != 2 { // 授权边 + 封装边各一条残留
		t.Fatalf("期望 2 条残留, 得到 %d: %+v", len(residuals), residuals)
	}
}

func TestFingerprintStable(t *testing.T) {
	_, eng, ctx := setupProof(t)
	fp1, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp1) != 64 {
		t.Fatalf("指纹长度应为 64, 得到 %d", len(fp1))
	}
	// 同一状态指纹应稳定。
	fp2, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("指纹应稳定: %s != %s", fp1, fp2)
	}
	// 摘要哈希确定。
	d1 := DigestOf(model.ProofSufficient, fp1, "s")
	d2 := DigestOf(model.ProofSufficient, fp1, "s")
	if d1 != d2 {
		t.Fatal("摘要应确定")
	}
}
