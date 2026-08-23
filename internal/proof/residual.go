package proof

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
)

// Residual 表示一条退休残留：已退休/已吊销密钥仍被授权边或
// 封装边引用。残留必须在计划完成前清空。
type Residual struct {
	Key      *model.Key         `json:"key"`
	Kind     string             `json:"kind"` // grant | wrap
	Subject  *model.Subject     `json:"subject,omitempty"`
	Object   *model.Object      `json:"object,omitempty"`
	Chain    *model.EvidenceChain `json:"chain"`
	Summary  string             `json:"summary"`
}

// ListResiduals 扫描全部退休/已吊销密钥的残留引用。
func (e *Engine) ListResiduals(ctx context.Context) ([]*Residual, error) {
	allKeys, err := e.keys.List(ctx, e.st.DB())
	if err != nil {
		return nil, err
	}
	var out []*Residual
	for _, k := range allKeys {
		if !k.Status.RetiredOrRevoked() {
			continue
		}
		res, err := e.ResidualsOfKey(ctx, k)
		if err != nil {
			return nil, err
		}
		out = append(out, res...)
	}
	return out, nil
}

// ResidualsOfKey 返回单个密钥的残留引用。
func (e *Engine) ResidualsOfKey(ctx context.Context, k *model.Key) ([]*Residual, error) {
	var out []*Residual
	grants, err := e.grants.GrantsOfKey(ctx, k.ID)
	if err != nil {
		return nil, err
	}
	for _, s := range grants {
		out = append(out, &Residual{
			Key:   k,
			Kind:  "grant",
			Subject: s,
			Chain: &model.EvidenceChain{
				Kind:   "residual",
				Length: 2,
				Nodes: []model.ChainNode{
					{Role: "subject", Name: s.Name, ID: s.ID},
					{Role: "retired", Name: "授权残留@" + k.Name, ID: k.ID},
				},
				Summary: fmt.Sprintf("已%s密钥 %s 仍被主体 %s 授权引用", k.Status, k.Name, s.Name),
			},
			Summary: fmt.Sprintf("授权边残留: 主体 %s -> 密钥 %s", s.Name, k.Name),
		})
	}
	wrapResiduals, err := e.wrapResiduals(ctx, k)
	if err != nil {
		return nil, err
	}
	out = append(out, wrapResiduals...)
	return out, nil
}

// wrapResiduals 找出封装边引用该密钥的对象。
func (e *Engine) wrapResiduals(ctx context.Context, k *model.Key) ([]*Residual, error) {
	allWraps, err := e.wraps.AllWraps(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Residual
	for _, w := range allWraps {
		if w.KeyID != k.ID {
			continue
		}
		obj, err := e.objects.Get(ctx, e.st.DB(), w.ObjectID)
		if err != nil {
			return nil, err
		}
		out = append(out, &Residual{
			Key:    k,
			Kind:   "wrap",
			Object: obj,
			Chain: &model.EvidenceChain{
				Kind:   "residual",
				Length: 2,
				Nodes: []model.ChainNode{
					{Role: "object", Name: obj.Name, ID: obj.ID},
					{Role: "retired", Name: "封装残留@" + k.Name, ID: k.ID},
				},
				Summary: fmt.Sprintf("已%s密钥 %s 仍封装对象 %s", k.Status, k.Name, obj.Name),
			},
			Summary: fmt.Sprintf("封装边残留: 对象 %s -> 密钥 %s", obj.Name, k.Name),
		})
	}
	return out, nil
}
