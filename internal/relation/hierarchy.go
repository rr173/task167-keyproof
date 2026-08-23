package relation

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// hierarchy 提供密钥层级的查询工具：祖先链、环检测、子树枚举。
type hierarchy struct {
	keys *store.KeyRepo
}

// Ancestors 返回 keyID 沿父链向上的祖先集合（含自身），
// 从 keyID 自身开始直到根密钥。集合顺序为 [key, parent, grandparent...]。
func (h *hierarchy) Ancestors(ctx context.Context, q store.Querier, keyID string) ([]*model.Key, error) {
	var chain []*model.Key
	cur := keyID
	seen := map[string]bool{}
	for cur != "" {
		if seen[cur] {
			return nil, fmt.Errorf("层级成环: %s", cur)
		}
		seen[cur] = true
		k, err := h.keys.Get(ctx, q, cur)
		if err != nil {
			return nil, err
		}
		chain = append(chain, k)
		cur = k.ParentID
	}
	return chain, nil
}

// AncestorIDs 返回祖先链的 ID 列表（含自身）。
func (h *hierarchy) AncestorIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	chain, err := h.Ancestors(ctx, q, keyID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(chain))
	for _, k := range chain {
		ids = append(ids, k.ID)
	}
	return ids, nil
}

// WouldCreateCycle 判断把 keyID 的父节点改为 newParent 是否成环。
// 成环条件：newParent 处于 keyID 的子树内，即从 newParent 沿父链向上
// 可达 keyID（无论中间隔着多少层，即任何间接成环）。
// 这同时覆盖了“把祖先挂到其后代下面”：若 keyID 已是 newParent 的祖先，
// 再让 keyID 认 newParent 为父，便会形成 keyID -> newParent -> ... -> keyID 的环。
func (h *hierarchy) WouldCreateCycle(ctx context.Context, q store.Querier, keyID, newParent string) (bool, error) {
	if newParent == "" {
		return false, nil
	}
	// 从 newParent 沿父链向上，若途经 keyID 则成环（含间接成环）。
	cur := newParent
	seen := map[string]bool{}
	for cur != "" {
		if cur == keyID {
			return true, nil
		}
		if seen[cur] {
			return false, fmt.Errorf("既有层级成环: %s", cur)
		}
		seen[cur] = true
		k, err := h.keys.Get(ctx, q, cur)
		if err != nil {
			return false, err
		}
		cur = k.ParentID
	}
	return false, nil
}

// SubtreeIDs 返回 keyID 及其全部子孙密钥 ID（BFS）。
func (h *hierarchy) SubtreeIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	all, err := h.keys.List(ctx, q)
	if err != nil {
		return nil, err
	}
	children := map[string][]string{}
	for _, k := range all {
		if k.ParentID != "" {
			children[k.ParentID] = append(children[k.ParentID], k.ID)
		}
	}
	queue := []string{keyID}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out = append(out, cur)
		queue = append(queue, children[cur]...)
	}
	return out, nil
}

// MoveKey 调整密钥层级（换父），带环检测与引用状态校验。
// 非根密钥必须拥有父节点（禁止换到空父）。
func (r *Registry) MoveKey(ctx context.Context, keyID, newParent string) error {
	k, err := r.keys.Get(ctx, r.st.DB(), keyID)
	if err != nil {
		return err
	}
	if k.Kind == model.KindRoot {
		return model.ErrInvalidState // 根密钥不允许换父
	}
	if newParent == "" {
		return model.ErrInvalidInput // 非根密钥必须挂在合法父节点下
	}
	if newParent == keyID {
		return model.ErrCycleDetected
	}
	parent, err := r.keys.Get(ctx, r.st.DB(), newParent)
	if err != nil {
		return err
	}
	if parent.Kind == model.KindData {
		return model.ErrInvalidInput // 数据密钥不能再有子密钥
	}
	if parent.Status.RetiredOrRevoked() {
		return model.ErrRetiredReference
	}
	h := &hierarchy{keys: r.keys}
	cycle, err := h.WouldCreateCycle(ctx, r.st.DB(), keyID, newParent)
	if err != nil {
		return err
	}
	if cycle {
		// 拒绝任何成环（含间接成环）：即便 keyID 与 newParent 之间
		// 已存在一条父链，也不得把祖先挂到其后代下面。
		return model.ErrCycleDetected
	}
	return r.keys.UpdateParent(ctx, r.st.DB(), keyID, newParent)
}

// hierarchyOf 供其它包获取层级查询工具。
func hierarchyOf(r *Registry) *hierarchy { return &hierarchy{keys: r.keys} }

// AncestorIDsOf 导出：返回 keyID 的祖先链 ID（含自身），
// 供 proof 等只读包使用。
func AncestorIDsOf(r *Registry, ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	return hierarchyOf(r).AncestorIDs(ctx, q, keyID)
}

// SubtreeIDsOf 导出：返回 keyID 及其全部子孙密钥 ID。
func SubtreeIDsOf(r *Registry, ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	return hierarchyOf(r).SubtreeIDs(ctx, q, keyID)
}
