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
	Keys     []*model.Key    `json:"keys"`
	Grants   []*model.Grant  `json:"grants"`
	Wraps    []*model.Wrap   `json:"wraps"`
	Objects  []*model.Object `json:"objects"`
	Subjects []*model.Subject `json:"subjects"`
}

// Fingerprint 计算当前输入边（密钥/授权/封装/对象/主体）的 SHA-256 指纹。
// 主体被纳入指纹，使主体生命周期状态（active/removed）的改动对
// 已完成步骤的证明保持校验可见——移除主体即令旧快照失效。
// 指纹用于证明快照的防改写校验与重启后的状态一致性比对。
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
	subjects, err := e.subjects.List(ctx, e.st.DB())
	if err != nil {
		return "", err
	}
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
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
	sort.SliceStable(subjects, func(i, j int) bool { return subjects[i].ID < subjects[j].ID })
	snap := InputSnapshot{Keys: keys, Grants: grants, Wraps: wraps, Objects: objects, Subjects: subjects}
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
