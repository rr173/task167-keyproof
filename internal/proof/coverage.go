// Package proof 计算密钥轮换覆盖证明：可解密覆盖集合、
// 最短缺口证据链、退休残留检测与输入指纹。
// 它是“每一步均满足覆盖与退休约束”的核心判定器。
package proof

import (
	"context"
	"fmt"
	"sort"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

// Engine 提供证明计算能力，只读关系图，不产生副作用。
type Engine struct {
	st       *store.Store
	grants   *relation.GrantService
	wraps    *relation.WrapService
	hier     hierarchyReader
	keys     *store.KeyRepo
	subjects *store.SubjectRepo
	objects  *store.ObjectRepo
	proofs   *store.ProofRepo
}

type hierarchyReader interface {
	AncestorIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error)
	SubtreeIDs(ctx context.Context, q store.Querier, keyID string) ([]string, error)
}

// NewEngine 构造证明引擎。
func NewEngine(st *store.Store, reg *relation.Registry) *Engine {
	return &Engine{
		st:       st,
		grants:   relation.GrantServiceOf(reg),
		wraps:    relation.WrapServiceOf(reg),
		hier:     hierarchyFrom(reg),
		keys:     store.NewKeyRepo(),
		subjects: store.NewSubjectRepo(),
		objects:  store.NewObjectRepo(),
		proofs:   store.NewProofRepo(),
	}
}

// CoveragePath 表示主体到对象的一条可解密路径。
type CoveragePath struct {
	SubjectID string   `json:"subjectId"`
	Subject   string   `json:"subject"`
	GrantKey  string   `json:"grantKey"`  // 授权点密钥
	WrapKey   string   `json:"wrapKey"`   // 封装密钥
	Length    int      `json:"length"`    // 边数
	AncestorChain []string `json:"ancestorChain"` // 授权点->封装点 之间的密钥
}

// CoverageOfObject 计算对象的全部可解密覆盖路径。
func (e *Engine) CoverageOfObject(ctx context.Context, objectID string) ([]*CoveragePath, error) {
	wrapKeys, err := e.wraps.WrapsOfObject(ctx, objectID)
	if err != nil {
		return nil, err
	}
	if len(wrapKeys) == 0 {
		return nil, nil // 无封装边：视为未覆盖
	}
	var paths []*CoveragePath
	for _, wk := range wrapKeys {
		// 沿封装密钥祖先链找被授权的最浅节点。
		chain, err := e.hier.AncestorIDs(ctx, e.st.DB(), wk.ID)
		if err != nil {
			return nil, err
		}
		for i, kID := range chain {
			subs, err := e.grants.GrantsOfKey(ctx, kID)
			if err != nil {
				return nil, err
			}
			for _, s := range subs {
				paths = append(paths, &CoveragePath{
					SubjectID:    s.ID,
					Subject:      s.Name,
					GrantKey:     kID,
					WrapKey:      wk.ID,
					Length:       1 + i + 1, // grant + 祖先边 + wrap
					AncestorChain: chain[:i],
				})
			}
		}
	}
	sort.SliceStable(paths, func(a, b int) bool {
		if paths[a].Length != paths[b].Length {
			return paths[a].Length < paths[b].Length
		}
		if paths[a].SubjectID != paths[b].SubjectID {
			return paths[a].SubjectID < paths[b].SubjectID
		}
		return paths[a].WrapKey < paths[b].WrapKey
	})
	return paths, nil
}

// IsOrphaned 判断对象是否孤立（不存在任何可解密主体）。
func (e *Engine) IsOrphaned(ctx context.Context, objectID string) (bool, error) {
	paths, err := e.CoverageOfObject(ctx, objectID)
	if err != nil {
		return false, err
	}
	return len(paths) == 0, nil
}

// SubjectCoverage 返回主体可解密的对象集合（供展示）。
func (e *Engine) SubjectCoverage(ctx context.Context, subjectID string) ([]string, error) {
	all, err := e.objects.List(ctx, e.st.DB())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, o := range all {
		paths, err := e.CoverageOfObject(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			if p.SubjectID == subjectID {
				out = append(out, o.ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// RootKeysWithoutAuth 返回无任何授权主体的根密钥（根链缺口）。
func (e *Engine) RootKeysWithoutAuth(ctx context.Context) ([]*model.Key, error) {
	all, err := e.keys.List(ctx, e.st.DB())
	if err != nil {
		return nil, err
	}
	var out []*model.Key
	for _, k := range all {
		if k.Kind != model.KindRoot {
			continue
		}
		subs, err := e.grants.GrantsOfKey(ctx, k.ID)
		if err != nil {
			return nil, err
		}
		if len(subs) == 0 {
			out = append(out, k)
		}
	}
	return out, nil
}

// orphanSummary 供摘要生成。
func orphanSummary(objectID string, wrapKeys []string) string {
	return fmt.Sprintf("对象 %s 无任何可解密主体（封装密钥: %v）", objectID, wrapKeys)
}
