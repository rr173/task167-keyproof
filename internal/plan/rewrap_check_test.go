package plan

import (
	"context"
	"errors"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/proof"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

// rewrapValidatorSetup 构造一个最小可用的验证环境：
// 根密钥(已授权)->数据密钥A(对象封装于此)->数据密钥B(重封装目标)。
func rewrapValidatorSetup(t *testing.T) (*Validator, *relation.Registry, context.Context) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	reg := relation.NewRegistry(st)
	engine := proof.NewEngine(st, reg)
	ctx := context.Background()

	root := &model.Key{ID: "r1", Name: "root", Kind: model.KindRoot, Status: model.KeyCandidate}
	if err := reg.CreateKey(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, err := reg.ActivateKey(ctx, "r1"); err != nil {
		t.Fatalf("activate root: %v", err)
	}
	dataA := &model.Key{ID: "dA", Name: "dataA", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, dataA); err != nil {
		t.Fatalf("create dataA: %v", err)
	}
	if _, err := reg.ActivateKey(ctx, "dA"); err != nil {
		t.Fatalf("activate dataA: %v", err)
	}
	dataB := &model.Key{ID: "dB", Name: "dataB", Kind: model.KindData, Status: model.KeyCandidate, ParentID: "r1"}
	if err := reg.CreateKey(ctx, dataB); err != nil {
		t.Fatalf("create dataB: %v", err)
	}
	if _, err := reg.ActivateKey(ctx, "dB"); err != nil {
		t.Fatalf("activate dataB: %v", err)
	}
	sub := &model.Subject{ID: "s1", Name: "eng", Status: model.SubjectActive}
	if err := reg.CreateSubject(ctx, sub); err != nil {
		t.Fatalf("create subject: %v", err)
	}
	if err := relation.GrantServiceOf(reg).AddGrant(ctx, "s1", "r1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	obj := &model.Object{ID: "o1", Name: "obj", Status: model.ObjectProtected}
	if err := reg.CreateObject(ctx, obj); err != nil {
		t.Fatalf("create object: %v", err)
	}
	if err := relation.WrapServiceOf(reg).AddWrap(ctx, "o1", "dA"); err != nil {
		t.Fatalf("wrap o1 to dA: %v", err)
	}
	return NewValidator(engine), reg, ctx
}

// TestRewrapBlockedWhenSourceWrapMissing 验证：对象未由指定源密钥封装时，
// 重新封装步骤必须被阻断，且现有封装关系保持不变。
func TestRewrapBlockedWhenSourceWrapMissing(t *testing.T) {
	v, reg, ctx := rewrapValidatorSetup(t)

	// 对象封装在 dA，但步骤声明的源密钥是 dB（不存在该封装边）。
	step := &model.PlanStep{
		ID:          "step-bad-source",
		Seq:         1,
		Action:      model.ActionRewrapObject,
		ObjectID:    "o1",
		KeyID:       "dB", // 源边不存在：对象并未由 dB 封装
		TargetKeyID: "dA",
	}
	res := v.Check(ctx, step, nil)
	if res.Err == nil {
		t.Fatalf("源封装边不存在时应阻断, 实际通过: %+v", res)
	}
	if !errors.Is(res.Err, model.ErrMissingWrap) {
		t.Fatalf("应返回 ErrMissingWrap, 实际 %v", res.Err)
	}

	// 现有封装关系必须保持：对象仍封装在 dA，未封装到 dB。
	ws := relation.WrapServiceOf(reg)
	keys, err := ws.WrapsOfObject(ctx, "o1")
	if err != nil {
		t.Fatalf("wraps of object: %v", err)
	}
	var ids []string
	for _, k := range keys {
		ids = append(ids, k.ID)
	}
	if !contains(ids, "dA") {
		t.Fatalf("现有封装边应保留 dA, 实际 %v", ids)
	}
	if contains(ids, "dB") {
		t.Fatalf("不应新增 dB 封装边, 实际 %v", ids)
	}
}

// TestRewrapPassesWhenSourceWrapExists 验证：对象确实由源密钥封装时，
// 重新封装步骤通过验证。
func TestRewrapPassesWhenSourceWrapExists(t *testing.T) {
	v, _, ctx := rewrapValidatorSetup(t)

	step := &model.PlanStep{
		ID:          "step-ok",
		Seq:         1,
		Action:      model.ActionRewrapObject,
		ObjectID:    "o1",
		KeyID:       "dA", // 源边存在
		TargetKeyID: "dB",
	}
	res := v.Check(ctx, step, nil)
	if res.Err != nil {
		t.Fatalf("源封装边存在时应通过, 实际阻断: %v", res.Err)
	}
}

// TestAddStepRewrapRequiresSourceKey 验证：追加重新封装步骤时，
// 源密钥(KeyID)不可为空。
func TestAddStepRewrapRequiresSourceKey(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	svc := NewService(st)
	p, err := svc.CreatePlan(ctx, "plan-rewrap", "rotation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddStep(ctx, p.ID, &model.PlanStep{
		ID: "step-1", Action: model.ActionRewrapObject,
		ObjectID: "o1", TargetKeyID: "dB", // 缺少 KeyID
	}); err != model.ErrInvalidInput {
		t.Fatalf("缺少源密钥应返回 ErrInvalidInput, 实际 %v", err)
	}
}

func contains(items []string, want string) bool {
	for _, x := range items {
		if x == want {
			return true
		}
	}
	return false
}
