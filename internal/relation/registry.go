// Package relation 维护密钥层级、授权边与对象封装边等关系。
// 它是整个服务的数据源：证明模块基于本包维护的图计算覆盖，
// 计划模块基于本包的可变操作设计轮换步骤。
package relation

import (
	"context"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// Registry 提供关系登记与变更的入口，所有写入都经过约束校验。
type Registry struct {
	keys     *store.KeyRepo
	subjects *store.SubjectRepo
	objects  *store.ObjectRepo
	st       *store.Store
}

// NewRegistry 构造关系登记服务。
func NewRegistry(st *store.Store) *Registry {
	return &Registry{
		keys:     store.NewKeyRepo(),
		subjects: store.NewSubjectRepo(),
		objects:  store.NewObjectRepo(),
		st:       st,
	}
}

// CreateKey 登记一个候选密钥。根密钥（root）允许无父；
// 非根密钥必须指定存在的父密钥。
func (r *Registry) CreateKey(ctx context.Context, k *model.Key) error {
	if !k.Kind.Valid() {
		return model.ErrInvalidInput
	}
	if !k.Status.Valid() {
		return model.ErrInvalidInput
	}
	if k.Kind != model.KindRoot && k.ParentID == "" {
		return model.ErrInvalidInput
	}
	if k.ParentID != "" {
		parent, err := r.keys.Get(ctx, r.st.DB(), k.ParentID)
		if err != nil {
			return err
		}
		if parent.Status.RetiredOrRevoked() {
			return model.ErrRetiredReference
		}
		// 数据密钥之下不应再有子密钥：层级限制为三层。
		if parent.Kind == model.KindData {
			return model.ErrInvalidInput
		}
	}
	return r.keys.Create(ctx, r.st.DB(), k)
}

// ActivateKey 启用候选密钥。只有 candidate 状态可启用。
func (r *Registry) ActivateKey(ctx context.Context, id string) (*model.Key, error) {
	k, err := r.keys.Get(ctx, r.st.DB(), id)
	if err != nil {
		return nil, err
	}
	if k.Status != model.KeyCandidate {
		return nil, model.ErrInvalidState
	}
	if err := r.keys.UpdateStatus(ctx, r.st.DB(), id, model.KeyActive); err != nil {
		return nil, err
	}
	k.Status = model.KeyActive
	return k, nil
}

// MarkRetiring 将启用密钥标记为退役待清理。只有 active 可进入。
func (r *Registry) MarkRetiring(ctx context.Context, id string) (*model.Key, error) {
	k, err := r.keys.Get(ctx, r.st.DB(), id)
	if err != nil {
		return nil, err
	}
	if k.Status != model.KeyActive {
		return nil, model.ErrInvalidState
	}
	if err := r.keys.UpdateStatus(ctx, r.st.DB(), id, model.KeyRetiring); err != nil {
		return nil, err
	}
	k.Status = model.KeyRetiring
	return k, nil
}

// RetireKey 将退役待清理密钥置为已退休。
// 只有 retiring 可退休；若仍存在引用边（授权/封装），
// 由 proof.Residual 负责检测，此处只做状态迁移。
func (r *Registry) RetireKey(ctx context.Context, id string) (*model.Key, error) {
	k, err := r.keys.Get(ctx, r.st.DB(), id)
	if err != nil {
		return nil, err
	}
	if k.Status != model.KeyRetiring {
		return nil, model.ErrInvalidState
	}
	if err := r.keys.UpdateStatus(ctx, r.st.DB(), id, model.KeyRetired); err != nil {
		return nil, err
	}
	k.Status = model.KeyRetired
	return k, nil
}

// RevokeKey 吊销密钥（active/retiring 可吊销，直接进入 revoked）。
func (r *Registry) RevokeKey(ctx context.Context, id string) (*model.Key, error) {
	k, err := r.keys.Get(ctx, r.st.DB(), id)
	if err != nil {
		return nil, err
	}
	if k.Status != model.KeyActive && k.Status != model.KeyRetiring {
		return nil, model.ErrInvalidState
	}
	if err := r.keys.UpdateStatus(ctx, r.st.DB(), id, model.KeyRevoked); err != nil {
		return nil, err
	}
	k.Status = model.KeyRevoked
	return k, nil
}

// CreateSubject 登记一个主体。
func (r *Registry) CreateSubject(ctx context.Context, s *model.Subject) error {
	if !s.Status.Valid() {
		return model.ErrInvalidInput
	}
	return r.subjects.Create(ctx, r.st.DB(), s)
}

// RemoveSubject 将主体置为已移除（removed）。
// 移除是软删除：历史授权边保留以供审计与退休残留检测，但主体不再
// 具备授权或解密能力——覆盖查询、最短证据链与孤立判定据此一致处理。
// 只有 active 主体可被移除；重复移除幂等返回当前主体。
func (r *Registry) RemoveSubject(ctx context.Context, id string) (*model.Subject, error) {
	s, err := r.subjects.Get(ctx, r.st.DB(), id)
	if err != nil {
		return nil, err
	}
	if s.Status == model.SubjectRemoved {
		return s, nil // 幂等
	}
	if s.Status != model.SubjectActive {
		return nil, model.ErrInvalidState
	}
	if err := r.subjects.UpdateStatus(ctx, r.st.DB(), id, model.SubjectRemoved); err != nil {
		return nil, err
	}
	s.Status = model.SubjectRemoved
	return s, nil
}

// CreateObject 登记一个受保护加密对象。
func (r *Registry) CreateObject(ctx context.Context, o *model.Object) error {
	if !o.Status.Valid() {
		return model.ErrInvalidInput
	}
	return r.objects.Create(ctx, r.st.DB(), o)
}
