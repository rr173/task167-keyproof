package proof

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"task167-keyproof/internal/model"
)

// InputSnapshot 是输入边快照：证明不可改写性的基础。
// 对已完成步骤重新计算指纹并与快照比对，即可发现篡改。
type InputSnapshot struct {
	Keys    []*model.Key    `json:"keys"`
	Grants  []*model.Grant  `json:"grants"`
	Wraps   []*model.Wrap   `json:"wraps"`
	Objects []*model.Object `json:"objects"`
}

// fpKey 是密钥参与指纹计算的规范视图：只保留身份与关系字段，
// 刻意排除 CreatedAt/UpdatedAt —— 后者由写入时刻决定，
// 同一组密钥以不同顺序登记时会有不同时间戳，从而污染指纹。
type fpKey struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     model.KeyKind   `json:"kind"`
	Status   model.KeyStatus `json:"status"`
	ParentID string          `json:"parentId,omitempty"`
}

// fpGrant 是授权边的规范视图：仅保留 (主体, 密钥) 二元组。
// 排除 CreatedAt 使指纹只取决于授权关系本身，与写入顺序/时刻无关。
type fpGrant struct {
	SubjectID string `json:"subjectId"`
	KeyID     string `json:"keyId"`
}

// fpWrap 是封装边的规范视图：仅保留 (对象, 密钥) 二元组，排除 CreatedAt。
type fpWrap struct {
	ObjectID string `json:"objectId"`
	KeyID    string `json:"keyId"`
}

// fpObject 是对象的规范视图：排除 CreatedAt/UpdatedAt。
type fpObject struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Status model.ObjectStatus `json:"status"`
}

// fpSnapshot 是参与指纹哈希的规范快照：每类实体只取身份/关系字段，
// 并按确定性顺序排列，使指纹与插入顺序及写入时刻无关。
type fpSnapshot struct {
	Keys    []fpKey    `json:"keys"`
	Grants  []fpGrant  `json:"grants"`
	Wraps   []fpWrap   `json:"wraps"`
	Objects []fpObject `json:"objects"`
}

// Fingerprint 计算当前输入边（密钥/授权/封装/对象）的 SHA-256 指纹。
// 指纹用于证明快照的防改写校验与重启后的状态一致性比对。
//
// 为保证“同一组授权主体与密钥关系无论以何种顺序写入都得到相同指纹”，
// 此处对每类实体先剔除随写入时刻变化的 CreatedAt/UpdatedAt 字段，
// 再按稳定的身份键排序后哈希；授权边按 (SubjectID, KeyID) 排序，
// 封装边按 (ObjectID, KeyID) 排序，密钥/对象按 ID 排序。
func (e *Engine) Fingerprint(ctx context.Context) (string, error) {
	keys, err := e.keys.List(ctx, e.st.DB())
	if err != nil {
		return "", err
	}
	grants, err := e.grants.AllGrants(ctx)
	if err != nil {
		return "", err
	}
	wraps, err := e.wraps.AllWraps(ctx)
	if err != nil {
		return "", err
	}
	objects, err := e.objects.List(ctx, e.st.DB())
	if err != nil {
		return "", err
	}
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	sort.SliceStable(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
	sort.SliceStable(grants, func(i, j int) bool {
		if grants[i].SubjectID != grants[j].SubjectID {
			return grants[i].SubjectID < grants[j].SubjectID
		}
		return grants[i].KeyID < grants[j].KeyID
	})
	sort.SliceStable(wraps, func(i, j int) bool {
		if wraps[i].ObjectID != wraps[j].ObjectID {
			return wraps[i].ObjectID < wraps[j].ObjectID
		}
		return wraps[i].KeyID < wraps[j].KeyID
	})
	snap := fpSnapshot{
		Keys:    make([]fpKey, 0, len(keys)),
		Grants:  make([]fpGrant, 0, len(grants)),
		Wraps:   make([]fpWrap, 0, len(wraps)),
		Objects: make([]fpObject, 0, len(objects)),
	}
	for _, k := range keys {
		snap.Keys = append(snap.Keys, fpKey{
			ID: k.ID, Name: k.Name, Kind: k.Kind, Status: k.Status, ParentID: k.ParentID,
		})
	}
	for _, g := range grants {
		snap.Grants = append(snap.Grants, fpGrant{SubjectID: g.SubjectID, KeyID: g.KeyID})
	}
	for _, w := range wraps {
		snap.Wraps = append(snap.Wraps, fpWrap{ObjectID: w.ObjectID, KeyID: w.KeyID})
	}
	for _, o := range objects {
		snap.Objects = append(snap.Objects, fpObject{ID: o.ID, Name: o.Name, Status: o.Status})
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// DigestOf 生成证明摘要：覆盖状态与输入指纹绑定。
func DigestOf(status model.ProofStatus, inputFP, summary string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(string(status)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(inputFP))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(summary))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyFingerprint 校验当前指纹与快照指纹一致。
func (e *Engine) VerifyFingerprint(ctx context.Context, expectFP string) (bool, error) {
	cur, err := e.Fingerprint(ctx)
	if err != nil {
		return false, err
	}
	return cur == expectFP, nil
}
