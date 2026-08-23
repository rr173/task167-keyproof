package proof

import (
	"context"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// SubtreeOf 返回 keyID 及其全部子孙密钥 ID（供退休阻断判定）。
func (e *Engine) SubtreeOf(ctx context.Context, keyID string) ([]string, error) {
	return e.hier.SubtreeIDs(ctx, e.st.DB(), keyID)
}

// AncestorsOf 返回 keyID 祖先链 ID（含自身）。
func (e *Engine) AncestorsOf(ctx context.Context, keyID string) ([]string, error) {
	return e.hier.AncestorIDs(ctx, e.st.DB(), keyID)
}

// KeyOf 查询密钥。
func (e *Engine) KeyOf(ctx context.Context, id string) (*model.Key, error) {
	return e.keys.Get(ctx, e.st.DB(), id)
}

// ObjectOf 查询对象。
func (e *Engine) ObjectOf(ctx context.Context, id string) (*model.Object, error) {
	return e.objects.Get(ctx, e.st.DB(), id)
}

// SubjectOf 查询主体。
func (e *Engine) SubjectOf(ctx context.Context, id string) (*model.Subject, error) {
	return e.subjects.Get(ctx, e.st.DB(), id)
}

// WrapsOfObject 返回对象的封装密钥列表。
func (e *Engine) WrapsOfObject(ctx context.Context, objectID string) ([]*model.Key, error) {
	return e.wraps.WrapsOfObject(ctx, objectID)
}

// GrantsOfKey 返回直接授权到某密钥的主体。
func (e *Engine) GrantsOfKey(ctx context.Context, keyID string) ([]*model.Subject, error) {
	return e.grants.GrantsOfKey(ctx, keyID)
}

// AuthorizedForKey 判断主体是否可解密指定密钥。
func (e *Engine) AuthorizedForKey(ctx context.Context, subjectID, keyID string) (bool, error) {
	return e.grants.AuthorizedForKey(ctx, subjectID, keyID)
}

// CoverageSubjects 返回对某密钥具有解密能力的主体集合。
func (e *Engine) CoverageSubjects(ctx context.Context, keyID string) ([]*model.Subject, error) {
	return e.grants.CoverageSubjects(ctx, keyID)
}

// Querier 透传 store.Querier 类型，供外部复用。
type Querier = store.Querier

// ListObjects 列出全部加密对象。
func (e *Engine) ListObjects(ctx context.Context) ([]*model.Object, error) {
	return e.objects.List(ctx, e.st.DB())
}

// ListKeys 列出全部密钥。
func (e *Engine) ListKeys(ctx context.Context) ([]*model.Key, error) {
	return e.keys.List(ctx, e.st.DB())
}

// ListSubjects 列出全部主体。
func (e *Engine) ListSubjects(ctx context.Context) ([]*model.Subject, error) {
	return e.subjects.List(ctx, e.st.DB())
}

// ListProofs 列出某计划的全部证明快照。
func (e *Engine) ListProofs(ctx context.Context, planID string) ([]*model.Proof, error) {
	return e.proofs.List(ctx, e.st.DB(), planID)
}

// ListAllProofs 列出全部证明快照。
func (e *Engine) ListAllProofs(ctx context.Context) ([]*model.Proof, error) {
	return e.proofs.ListAll(ctx, e.st.DB())
}

// SaveProof 保存证明快照。
func (e *Engine) SaveProof(ctx context.Context, p *model.Proof) error {
	return e.proofs.Save(ctx, e.st.DB(), p)
}

// GetProofByStep 按计划与步骤查询证明快照。
func (e *Engine) GetProofByStep(ctx context.Context, planID, stepID string) (*model.Proof, error) {
	return e.proofs.GetByStep(ctx, e.st.DB(), planID, stepID)
}
