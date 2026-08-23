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

// buildFingerprintFixture 在独立库内登记一组固定的输入边（根密钥 +
// 主体授权 + 多个对象封装于同一组数据密钥），返回其覆盖指纹。
// order 决定对象与封装边的登记顺序，用于验证落库顺序无关性。
func buildFingerprintFixture(t *testing.T, order []string) string {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/fp.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	reg := relation.NewRegistry(st)
	eng := NewEngine(st, reg)
	ctx := context.Background()

	// 密钥层级：root -> data；二者均启用。
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, err := reg.ActivateKey(ctx, "r1"); err != nil {
		t.Fatalf("activate root: %v", err)
	}
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, data); err != nil {
		t.Fatalf("create data: %v", err)
	}
	if _, err := reg.ActivateKey(ctx, "d1"); err != nil {
		t.Fatalf("activate data: %v", err)
	}
	// 主体对根密钥授权（根链有效，覆盖成立）。
	sub := &model.Subject{ID: "s1", Name: "eng", Status: model.SubjectActive}
	if err := reg.CreateSubject(ctx, sub); err != nil {
		t.Fatalf("create subject: %v", err)
	}
	if err := relation.GrantServiceOf(reg).AddGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// 按给定顺序登记同一组对象并封装到 d1。
	for _, id := range order {
		o := &model.Object{ID: id, Name: id, Status: model.ObjectProtected}
		if err := reg.CreateObject(ctx, o); err != nil {
			t.Fatalf("create object %s: %v", id, err)
		}
		if err := relation.WrapServiceOf(reg).AddWrap(ctx, id, "d1"); err != nil {
			t.Fatalf("wrap %s: %v", id, err)
		}
	}
	fp, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return fp
}

// TestFingerprintInvariantUnderInsertionOrder 证明输入集合的插入顺序
// 不应影响覆盖指纹：同一组封装边即使经历不同的落库顺序，
// 也必须得到相同的指纹结果。
func TestFingerprintInvariantUnderInsertionOrder(t *testing.T) {
	ids := []string{"o1", "o2", "o3", "o4"}
	// 正序落库。
	fpForward := buildFingerprintFixture(t, ids)
	// 逆序落库：完全相同的输入边集合，但落库顺序不同。
	rev := make([]string, len(ids))
	for i, id := range ids {
		rev[len(ids)-1-i] = id
	}
	fpReverse := buildFingerprintFixture(t, rev)
	if fpForward != fpReverse {
		t.Fatalf("插入顺序不应影响覆盖指纹: 正序=%s 逆序=%s", fpForward, fpReverse)
	}

	// 交叉顺序：同集合的另一落库顺序。
	fpShuffled := buildFingerprintFixture(t, []string{"o3", "o1", "o4", "o2"})
	if fpShuffled != fpForward {
		t.Fatalf("插入顺序不应影响覆盖指纹: 正序=%s 交叉序=%s", fpForward, fpShuffled)
	}

	// 指纹长度仍为 64（SHA-256 hex）。
	if len(fpForward) != 64 {
		t.Fatalf("指纹长度应为 64, 得到 %d", len(fpForward))
	}
}

// TestFingerprintSensitiveToContent 断言顺序无关不等于内容无关：
// 同一顺序下增删任一封装边必须改变指纹（防止排序修复误伤篡改检测）。
func TestFingerprintSensitiveToContent(t *testing.T) {
	base := buildFingerprintFixture(t, []string{"o1", "o2"})

	st, err := store.Open(t.TempDir() + "/fp2.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	reg := relation.NewRegistry(st)
	eng := NewEngine(st, reg)
	ctx := context.Background()
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	_ = reg.CreateKey(ctx, data)
	_, _ = reg.ActivateKey(ctx, "d1")
	sub := &model.Subject{ID: "s1", Name: "eng", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, sub)
	_ = relation.GrantServiceOf(reg).AddGrant(ctx, "s1", "r1")
	// 同顺序登记 o1、o2，再额外多登记一个 o3：输入集合不同，指纹必须不同。
	for _, id := range []string{"o1", "o2", "o3"} {
		_ = reg.CreateObject(ctx, &model.Object{ID: id, Name: id, Status: model.ObjectProtected})
		if err := relation.WrapServiceOf(reg).AddWrap(ctx, id, "d1"); err != nil {
			t.Fatalf("wrap %s: %v", id, err)
		}
	}
	more, err := eng.Fingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if more == base {
		t.Fatal("增删封装边后指纹应变化")
	}
}
