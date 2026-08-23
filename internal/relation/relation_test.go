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

// TestRewrapMarksObjectRewrapped 验证对象重新封装成功后状态进入 rewrapped，
// 后续查询可据此识别该对象已完成重新封装。
func TestRewrapMarksObjectRewrapped(t *testing.T) {
	st, reg, ctx := setup(t)
	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	_, _ = reg.ActivateKey(ctx, "r1")
	data := &model.Key{ID: "d1", Name: "data", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, data); err != nil {
		t.Fatalf("create data: %v", err)
	}
	_, _ = reg.ActivateKey(ctx, "d1")
	data2 := &model.Key{ID: "d2", Name: "data2", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, data2); err != nil {
		t.Fatalf("create data2: %v", err)
	}
	_, _ = reg.ActivateKey(ctx, "d2")

	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatalf("create object: %v", err)
	}
	ws := WrapServiceOf(reg)
	if err := ws.AddWrap(ctx, "o1", "d1"); err != nil {
		t.Fatalf("wrap: %v", err)
	}

	// 重新封装成功后，对象状态必须进入 rewrapped。
	if err := ws.Rewrap(ctx, "o1", "d1", "d2"); err != nil {
		t.Fatalf("rewrap: %v", err)
	}
	objects := store.NewObjectRepo()
	got, err := objects.Get(ctx, st.DB(), "o1")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if got.Status != model.ObjectRewrapped {
		t.Fatalf("重封装后对象状态应为 rewrapped, 实际 %s", got.Status)
	}
	// 反向重封装（回滚方向）同样落于 rewrapped。
	if err := ws.Rewrap(ctx, "o1", "d2", "d1"); err != nil {
		t.Fatalf("rewrap back: %v", err)
	}
	got, err = objects.Get(ctx, st.DB(), "o1")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if got.Status != model.ObjectRewrapped {
		t.Fatalf("反向重封装后对象状态应为 rewrapped, 实际 %s", got.Status)
	}
}
