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

// TestRemovedSubjectExcludedFromCoverage 验证主体被移除后：
//   - 不再出现在对象覆盖证明（CoverageOfObject）中；
//   - 不再被判为可解密对象（ShortestPath 给出缺口链而非覆盖链）；
//   - 仅由该主体覆盖的对象被判定为孤立（IsOrphaned）。
// 覆盖查询与最短证据链对主体生命周期状态保持一致处理。
func TestRemovedSubjectExcludedFromCoverage(t *testing.T) {
	st, eng, ctx := setupProof(t)
	reg := relation.NewRegistry(st)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	sub := &model.Subject{ID: "s1", Name: "eng", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, sub)
	gs := relation.GrantServiceOf(reg)
	if err := gs.AddGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)
	ws := relation.WrapServiceOf(reg)
	if err := ws.AddWrap(ctx, "o1", "r1"); err != nil {
		t.Fatalf("wrap: %v", err)
	}

	// 移除前：s1 覆盖 o1，可解密，对象非孤立。
	paths, err := eng.CoverageOfObject(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].SubjectID != "s1" {
		t.Fatalf("移除前应仅 s1 覆盖 o1: %+v", paths)
	}
	orphaned, err := eng.IsOrphaned(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if orphaned {
		t.Fatal("移除前 o1 不应孤立")
	}

	// 移除主体（软删除，授权边保留）。
	if _, err := reg.RemoveSubject(ctx, "s1"); err != nil {
		t.Fatalf("remove subject: %v", err)
	}

	// 移除后：o1 覆盖证明为空。
	paths, err = eng.CoverageOfObject(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if p.SubjectID == "s1" {
			t.Fatalf("移除主体 s1 不应继续出现在覆盖证明中: %+v", paths)
		}
	}
	// 仅由 s1 覆盖的对象现应孤立。
	orphaned, err = eng.IsOrphaned(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	if !orphaned {
		t.Fatal("移除覆盖主体后 o1 应被判为孤立")
	}
	// 最短证据链：s1 不再被判为可解密对象（缺口链而非覆盖链）。
	chain, err := eng.ShortestPath(ctx, "s1", "o1")
	if err != nil {
		t.Fatalf("shortest path: %v", err)
	}
	if chain == nil || chain.Kind != "gap" {
		t.Fatalf("移除主体应得到缺口链, 得到 %+v", chain)
	}
	// AuthorizedForKey：移除主体不可解密任何密钥。
	gs2 := relation.GrantServiceOf(reg)
	ok, err := gs2.AuthorizedForKey(ctx, "s1", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("移除主体不应被判为可解密")
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
