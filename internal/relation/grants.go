package relation

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// GrantService 维护主体 -> 密钥的授权边。
type GrantService struct {
	subjects *store.SubjectRepo
	keys     *store.KeyRepo
	st       *store.Store
	h        *hierarchy
}

// GrantServiceOf 从 Registry 提取授权服务。
func GrantServiceOf(r *Registry) *GrantService {
	return &GrantService{subjects: r.subjects, keys: r.keys, st: r.st, h: hierarchyOf(r)}
}

// AddGrant 为主体新增对某密钥的授权。
// 约束：主体存在；密钥存在且未退休/吊销；根密钥授权后即满足
// “根链有授权主体”的登记要求。
func (g *GrantService) AddGrant(ctx context.Context, subjectID, keyID string) error {
	return g.AddGrantTx(ctx, g.st.DB(), subjectID, keyID)
}

// AddGrantTx 事务版授权。
func (g *GrantService) AddGrantTx(ctx context.Context, q store.Querier, subjectID, keyID string) error {
	if _, err := g.subjects.Get(ctx, q, subjectID); err != nil {
		return err
	}
	k, err := g.keys.Get(ctx, q, keyID)
	if err != nil {
		return err
	}
	if k.Status.RetiredOrRevoked() {
		return model.ErrRetiredReference // 拒绝已退休密钥的新授权
	}
	grant := &model.Grant{SubjectID: subjectID, KeyID: keyID}
	if err := g.subjects.AddGrant(ctx, q, grant); err != nil {
		return err
	}
	return nil
}

// RevokeGrant 撤销授权边。
func (g *GrantService) RevokeGrant(ctx context.Context, subjectID, keyID string) error {
	return g.RevokeGrantTx(ctx, g.st.DB(), subjectID, keyID)
}

// RevokeGrantTx 事务版撤销授权。
func (g *GrantService) RevokeGrantTx(ctx context.Context, q store.Querier, subjectID, keyID string) error {
	return g.subjects.RemoveGrant(ctx, q, subjectID, keyID)
}

// GrantsOfKey 返回直接授权到某密钥的全部主体。
// 移除（removed）的主体不再具备解密能力，故不返回——覆盖查询、
// 最短证据链与孤立判定据此对主体生命周期状态一致处理。
func (g *GrantService) GrantsOfKey(ctx context.Context, keyID string) ([]*model.Subject, error) {
	grants, err := g.subjects.ListGrants(ctx, g.st.DB(), "", keyID)
	if err != nil {
		return nil, err
	}
	var out []*model.Subject
	for _, gr := range grants {
		s, err := g.subjects.Get(ctx, g.st.DB(), gr.SubjectID)
		if err != nil {
			return nil, err
		}
		if s.Status != model.SubjectActive {
			continue // 已移除主体：不再授权、不再可解密
		}
		out = append(out, s)
	}
	return out, nil
}

// GrantsOfSubject 返回主体直接授权的全部密钥。
// 已移除的主体不再持有任何授权能力，返回空集。
func (g *GrantService) GrantsOfSubject(ctx context.Context, subjectID string) ([]*model.Key, error) {
	s, err := g.subjects.Get(ctx, g.st.DB(), subjectID)
	if err != nil {
		return nil, err
	}
	if s.Status != model.SubjectActive {
		return nil, nil // 移除主体：不再持有授权
	}
	grants, err := g.subjects.ListGrants(ctx, g.st.DB(), subjectID, "")
	if err != nil {
		return nil, err
	}
	var out []*model.Key
	for _, gr := range grants {
		k, err := g.keys.Get(ctx, g.st.DB(), gr.KeyID)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// AuthorizedForKey 判断主体是否可直接解密指定密钥（即授权到
// 该密钥或该密钥的任一祖先）。已移除的主体不可解密任何密钥。
func (g *GrantService) AuthorizedForKey(ctx context.Context, subjectID, keyID string) (bool, error) {
	s, err := g.subjects.Get(ctx, g.st.DB(), subjectID)
	if err != nil {
		return false, err
	}
	if s.Status != model.SubjectActive {
		return false, nil // 移除主体：不具备解密能力
	}
	chain, err := g.h.AncestorIDs(ctx, g.st.DB(), keyID)
	if err != nil {
		return false, err
	}
	for _, kID := range chain {
		grants, err := g.subjects.ListGrants(ctx, g.st.DB(), subjectID, kID)
		if err != nil {
			return false, err
		}
		if len(grants) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// CoverageSubjects 返回对某密钥（含其子孙封装对象）具有解密能力的
// 全部主体集合：授权到该密钥或其任一祖先的主体。
func (g *GrantService) CoverageSubjects(ctx context.Context, keyID string) ([]*model.Subject, error) {
	ancestors, err := g.h.AncestorIDs(ctx, g.st.DB(), keyID)
	if err != nil {
		return nil, err
	}
	seen := map[string]*model.Subject{}
	var order []*model.Subject
	for _, kID := range ancestors {
		subs, err := g.GrantsOfKey(ctx, kID)
		if err != nil {
			return nil, err
		}
		for _, s := range subs {
			if _, ok := seen[s.ID]; !ok {
				seen[s.ID] = s
				order = append(order, s)
			}
		}
	}
	return order, nil
}

// AllGrants 返回全部授权边（供证明指纹计算）。
func (g *GrantService) AllGrants(ctx context.Context) ([]*model.Grant, error) {
	return g.subjects.ListGrants(ctx, g.st.DB(), "", "")
}

// describe 用于错误信息。
func describeKey(k *model.Key) string {
	if k == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s(%s)", k.Name, k.ID)
}
