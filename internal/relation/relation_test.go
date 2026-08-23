package relation

import (
	"context"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

func setup(t *testing.T) (*store.Store, *Registry, context.Context) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st, NewRegistry(st), context.Background()
}

func TestHierarchyAncestors(t *testing.T) {
	st, reg, ctx := setup(t)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	tenant := &model.Key{ID: "t1", Name: "tenant", Kind: model.KindTenant, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "t1"}
	if err := reg.CreateKey(ctx, data); err != nil {
		t.Fatalf("create data: %v", err)
	}
	h := hierarchyOf(reg)
	chain, err := h.AncestorIDs(ctx, st.DB(), "d1")
	if err != nil {
		t.Fatalf("ancestors: %v", err)
	}
	want := []string{"d1", "t1", "r1"}
	if len(chain) != len(want) {
		t.Fatalf("chain len = %d, want %d: %v", len(chain), len(want), chain)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("chain[%d] = %s, want %s", i, chain[i], want[i])
		}
	}
}

func TestHierarchyCycleRejected(t *testing.T) {
	_, reg, ctx := setup(t)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	tenant := &model.Key{ID: "t1", Name: "tenant", Kind: model.KindTenant, Status: model.KeyCandidate, ParentID: "r1"}
	_ = reg.CreateKey(ctx, tenant)
	// 数据密钥不能再有子密钥。
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "t1"}
	_ = reg.CreateKey(ctx, data)
	if err := reg.CreateKey(ctx, &model.Key{ID: "d2", Name: "d2", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "d1"}); err != model.ErrInvalidInput {
		t.Fatalf("数据密钥下挂子密钥应拒绝, got %v", err)
	}
	// 根密钥不允许换父。
	if err := reg.MoveKey(ctx, "r1", "t1"); err != model.ErrInvalidState {
		t.Fatalf("根密钥换父应拒绝, got %v", err)
	}
	// 非根密钥换空父应拒绝。
	if err := reg.MoveKey(ctx, "t1", ""); err != model.ErrInvalidInput {
		t.Fatalf("非根密钥换空父应拒绝, got %v", err)
	}
}

func TestGrantAndWrapConstraints(t *testing.T) {
	_, reg, ctx := setup(t)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	sub := &model.Subject{ID: "s1", Name: "engineer", Status: model.SubjectActive}
	_ = reg.CreateSubject(ctx, sub)
	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)

	gs := GrantServiceOf(reg)
	if err := gs.AddGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	ws := WrapServiceOf(reg)
	if err := ws.AddWrap(ctx, "o1", "r1"); err != nil {
		t.Fatalf("wrap: %v", err)
	}
	// 授权传播：s1 可解密 r1 本身。
	ok, err := gs.AuthorizedForKey(ctx, "s1", "r1")
	if err != nil || !ok {
		t.Fatalf("authorized: %v %v", ok, err)
	}
	// 撤销授权。
	if err := gs.RevokeGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ok, _ = gs.AuthorizedForKey(ctx, "s1", "r1")
	if ok {
		t.Fatal("撤销后不应再授权")
	}
}

func TestWrapRejectsRetiredKey(t *testing.T) {
	_, reg, ctx := setup(t)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	_, _ = reg.MarkRetiring(ctx, "r1")
	_, _ = reg.RetireKey(ctx, "r1")
	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)
	ws := WrapServiceOf(reg)
	if err := ws.AddWrap(ctx, "o1", "r1"); err != model.ErrRetiredReference {
		t.Fatalf("已退休密钥应拒绝新封装, got %v", err)
	}
}

// TestWrapRejectsRetiringKey 验证退役待清理（retiring）状态的密钥
// 不能再接收新的对象封装边，且拒绝写入不会污染对象已有的封装关系。
func TestWrapRejectsRetiringKey(t *testing.T) {
	st, reg, ctx := setup(t)
	// 两层密钥：dataA（将进入退役待清理）与 dataB（保持 active，作为既有封装边）。
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	_ = reg.CreateKey(ctx, root)
	_, _ = reg.ActivateKey(ctx, "r1")
	tenant := &model.Key{ID: "t1", Name: "tenant", Kind: model.KindTenant, Status: model.KeyCandidate, ParentID: "r1"}
	_ = reg.CreateKey(ctx, tenant)
	_, _ = reg.ActivateKey(ctx, "t1")
	dataA := &model.Key{ID: "d1", Name: "dataA", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "t1"}
	_ = reg.CreateKey(ctx, dataA)
	_, _ = reg.ActivateKey(ctx, "d1")
	dataB := &model.Key{ID: "d2", Name: "dataB", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "t1"}
	_ = reg.CreateKey(ctx, dataB)
	_, _ = reg.ActivateKey(ctx, "d2")

	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	_ = reg.CreateObject(ctx, obj)
	ws := WrapServiceOf(reg)
	// 既有封装边：对象已封装在 dataB 上。
	if err := ws.AddWrap(ctx, "o1", "d2"); err != nil {
		t.Fatalf("建立既有封装边: %v", err)
	}
	// dataA 进入退役待清理。
	if _, err := reg.MarkRetiring(ctx, "d1"); err != nil {
		t.Fatalf("标记退役待清理: %v", err)
	}
	// 新写入到退役待清理密钥应被拒绝。
	if err := ws.AddWrap(ctx, "o1", "d1"); err != model.ErrRetiredReference {
		t.Fatalf("退役待清理密钥应拒绝新封装, got %v", err)
	}
	// 拒绝写入不应污染既有封装关系：对象仍只由 dataB 封装。
	ids, err := reg.objects.WrapKeyIDs(ctx, st.DB(), "o1")
	if err != nil {
		t.Fatalf("查询封装边: %v", err)
	}
	if len(ids) != 1 || ids[0] != "d2" {
		t.Fatalf("既有封装关系被污染, got %v", ids)
	}
	// 事务版同样拒绝，且不留下半截写入。
	txErr := st.WithTx(func(tx *store.Tx) error {
		return ws.AddWrapTx(ctx, tx, "o1", "d1")
	})
	if txErr != model.ErrRetiredReference {
		t.Fatalf("事务版退役待清理密钥应拒绝新封装, got %v", txErr)
	}
	ids2, _ := reg.objects.WrapKeyIDs(ctx, st.DB(), "o1")
	if len(ids2) != 1 || ids2[0] != "d2" {
		t.Fatalf("事务版拒绝后既有封装关系被污染, got %v", ids2)
	}
}
