package proof

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

// hierarchyFrom 从 Registry 提取层级读取器。
func hierarchyFrom(reg *relation.Registry) hierarchyReader {
	return &hierAdapter{reg: reg}
}

type hierAdapter struct{ reg *relation.Registry }

func (h *hierAdapter) AncestorIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	return relation.AncestorIDsOf(h.reg, ctx, q, keyID)
}

func (h *hierAdapter) SubtreeIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error) {
	return relation.SubtreeIDsOf(h.reg, ctx, q, keyID)
}

// ShortestPath 计算主体 subjectID 到对象 objectID 的最短覆盖证据链。
// 可达时返回 kind=coverage 的链；不可达时返回 kind=gap 的
// 最短缺口链：对象经封装密钥沿祖先链到最近有授权节点的路径。
func (e *Engine) ShortestPath(ctx context.Context, subjectID, objectID string) (*model.EvidenceChain, error) {
	obj, err := e.objects.Get(ctx, e.st.DB(), objectID)
	if err != nil {
		return nil, err
	}
	sub, err := e.subjects.Get(ctx, e.st.DB(), subjectID)
	if err != nil {
		return nil, err
	}
	wrapKeys, err := e.wraps.WrapsOfObject(ctx, objectID)
	if err != nil {
		return nil, err
	}
	if len(wrapKeys) == 0 {
		return gapChain(sub, obj, "对象没有任何封装密钥"), nil
	}

	// 第一遍：寻找最短覆盖路径。
	best := (*model.EvidenceChain)(nil)
	for _, wk := range wrapKeys {
		chain, err := e.hier.AncestorIDs(ctx, e.st.DB(), wk.ID)
		if err != nil {
			return nil, err
		}
		for i, kID := range chain {
			grants, err := e.grants.GrantsOfKey(ctx, kID)
			if err != nil {
				return nil, err
			}
			for _, gs := range grants {
				if gs.ID == subjectID {
					c := buildCoverageChain(sub, obj, wk, kID, chain, i)
					if best == nil || c.Length < best.Length {
						best = c
					}
				}
			}
		}
	}
	if best != nil {
		return best, nil
	}

	// 不可达：构造最短缺口链——对每个封装密钥，找祖先链上
	// 最近的有授权节点；距离最短的即缺口位置。
	var nearest *model.EvidenceChain
	nearestDist := -1
	for _, wk := range wrapKeys {
		chain, err := e.hier.AncestorIDs(ctx, e.st.DB(), wk.ID)
		if err != nil {
			return nil, err
		}
		for i, kID := range chain {
			grants, err := e.grants.GrantsOfKey(ctx, kID)
			if err != nil {
				return nil, err
			}
			if len(grants) == 0 {
				continue
			}
			if nearestDist < 0 || i < nearestDist {
				nearestDist = i
				nearest = buildGapChain(sub, obj, wk, chain, i)
			}
			break // 该封装密钥链已找到最近授权点，不再向上
		}
	}
	if nearest == nil {
		// 整条链都没有授权：报告根链缺口。
		chain, err := e.hier.AncestorIDs(ctx, e.st.DB(), wrapKeys[0].ID)
		if err != nil {
			return nil, err
		}
		nearest = buildGapChain(sub, obj, wrapKeys[0], chain, len(chain)-1)
	}
	return nearest, nil
}

// buildCoverageChain 构造主体 -> 授权点 -> 祖先链 -> 封装密钥 -> 对象
// 的覆盖证据链。grantIdx 为授权点在祖先链中的下标（0 为封装密钥自身）。
func buildCoverageChain(sub *model.Subject, obj *model.Object, wrapKey *model.Key, grantKey string, ancestorChain []string, grantIdx int) *model.EvidenceChain {
	nodes := []model.ChainNode{
		{Role: "subject", Name: sub.Name, ID: sub.ID},
		{Role: "grant", Name: "授权@" + grantKey, ID: grantKey},
	}
	// ancestorChain[0] 是封装密钥自身，[grantIdx] 是授权点。
	// 中间元素为授权点之下的各层密钥。
	for i := grantIdx - 1; i >= 0; i-- {
		nodes = append(nodes, model.ChainNode{Role: "ancestor", Name: ancestorChain[i], ID: ancestorChain[i]})
	}
	nodes = append(nodes,
		model.ChainNode{Role: "key", Name: wrapKey.Name, ID: wrapKey.ID},
		model.ChainNode{Role: "wrap", Name: "封装", ID: wrapKey.ID},
		model.ChainNode{Role: "object", Name: obj.Name, ID: obj.ID},
	)
	return &model.EvidenceChain{
		Kind:    "coverage",
		Length:  1 + grantIdx + 1,
		Nodes:   nodes,
		Summary: fmt.Sprintf("主体 %s 经密钥 %s 可解密对象 %s", sub.Name, wrapKey.Name, obj.Name),
	}
}

// buildGapChain 构造对象到最近授权节点的缺口证据链。
// gapIdx 为最近有授权节点的祖先链下标；其下方（更靠近对象）
// 的密钥链即缺口所在。
func buildGapChain(sub *model.Subject, obj *model.Object, wrapKey *model.Key, ancestorChain []string, gapIdx int) *model.EvidenceChain {
	nodes := []model.ChainNode{
		{Role: "object", Name: obj.Name, ID: obj.ID},
		{Role: "wrap", Name: "封装", ID: wrapKey.ID},
	}
	// 从封装密钥向上到缺口位置（不含已授权的 gapIdx 节点）。
	for i := 0; i < gapIdx; i++ {
		nodes = append(nodes, model.ChainNode{Role: "gap", Name: "无授权@" + ancestorChain[i], ID: ancestorChain[i]})
	}
	nodes = append(nodes,
		model.ChainNode{Role: "key", Name: "最近授权@" + ancestorChain[gapIdx], ID: ancestorChain[gapIdx]},
		model.ChainNode{Role: "gap", Name: "授权链断裂", ID: ""},
		model.ChainNode{Role: "subject", Name: sub.Name, ID: sub.ID},
	)
	return &model.EvidenceChain{
		Kind:    "gap",
		Length:  gapIdx + 3,
		Nodes:   nodes,
		Summary: fmt.Sprintf("主体 %s 无法解密对象 %s：密钥 %s 至 %s 之间无授权", sub.Name, obj.Name, ancestorChain[gapIdx], wrapKey.ID),
	}
}

// gapChain 构造无封装边时的缺口链。
func gapChain(sub *model.Subject, obj *model.Object, reason string) *model.EvidenceChain {
	nodes := []model.ChainNode{
		{Role: "subject", Name: sub.Name, ID: sub.ID},
		{Role: "gap", Name: "无封装", ID: ""},
		{Role: "object", Name: obj.Name, ID: obj.ID},
	}
	return &model.EvidenceChain{
		Kind:    "gap",
		Length:  1,
		Nodes:   nodes,
		Summary: fmt.Sprintf("主体 %s 无法解密对象 %s：%s", sub.Name, obj.Name, reason),
	}
}
