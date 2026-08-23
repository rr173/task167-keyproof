package relation

import (
	"context"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/store"
)

// WrapService 维护对象 -> 密钥的封装边。
type WrapService struct {
	objects *store.ObjectRepo
	keys    *store.KeyRepo
	st      *store.Store
}

// WrapServiceOf 从 Registry 提取封装服务。
func WrapServiceOf(r *Registry) *WrapService {
	return &WrapService{objects: r.objects, keys: r.keys, st: r.st}
}

// AddWrap 为对象绑定封装边（对象被密钥加密）。
// 约束：对象存在；密钥存在且状态为 active（candidate 未启用、
// retiring/retired/revoked 均不可新增引用）。
func (w *WrapService) AddWrap(ctx context.Context, objectID, keyID string) error {
	return w.AddWrapTx(ctx, w.st.DB(), objectID, keyID)
}

// AddWrapTx 事务版封装边绑定。
func (w *WrapService) AddWrapTx(ctx context.Context, q store.Querier, objectID, keyID string) error {
	if _, err := w.objects.Get(ctx, q, objectID); err != nil {
		return err
	}
	k, err := w.keys.Get(ctx, q, keyID)
	if err != nil {
		return err
	}
	if k.Status != model.KeyActive {
		if k.Status.RetiredOrRevoked() {
			return model.ErrRetiredReference // 拒绝已退休密钥的新写入
		}
		return model.ErrInvalidState // candidate/retiring 不可建立新封装
	}
	wrap := &model.Wrap{ObjectID: objectID, KeyID: keyID}
	return w.objects.AddWrap(ctx, q, wrap)
}

// RemoveWrap 移除封装边。
func (w *WrapService) RemoveWrap(ctx context.Context, objectID, keyID string) error {
	return w.RemoveWrapTx(ctx, w.st.DB(), objectID, keyID)
}

// RemoveWrapTx 事务版移除封装边。
func (w *WrapService) RemoveWrapTx(ctx context.Context, q store.Querier, objectID, keyID string) error {
	return w.objects.RemoveWrap(ctx, q, objectID, keyID)
}

// Rewrap 将对象从旧密钥重新封装到新密钥：
// 新增 对象->新密钥 边并移除 对象->旧密钥 边（单事务原子完成）。
func (w *WrapService) Rewrap(ctx context.Context, objectID, oldKeyID, newKeyID string) error {
	return w.st.WithTx(func(tx *store.Tx) error {
		return w.RewrapTx(ctx, tx, objectID, oldKeyID, newKeyID)
	})
}

// RewrapTx 在给定事务内执行重新封装。
// 成功后对象状态进入 rewrapped：表示已完成重新封装，
// 证明与重启恢复流程可据此识别该对象已完成迁移。
// （对象状态机为 protected -> migrating -> rewrapped；本实现中
// 重封装为单事务原子完成，故直接落于终态 rewrapped。）
func (w *WrapService) RewrapTx(ctx context.Context, q store.Querier, objectID, oldKeyID, newKeyID string) error {
	if _, err := w.objects.Get(ctx, q, objectID); err != nil {
		return err
	}
	if newKeyID == oldKeyID {
		return model.ErrInvalidInput
	}
	nk, err := w.keys.Get(ctx, q, newKeyID)
	if err != nil {
		return err
	}
	if nk.Status != model.KeyActive {
		return model.ErrInvalidState
	}
	ok, err := w.keys.Get(ctx, q, oldKeyID)
	if err != nil {
		return err
	}
	if ok.Status.RetiredOrRevoked() {
		return model.ErrRetiredReference
	}
	if err := w.objects.AddWrap(ctx, q, &model.Wrap{ObjectID: objectID, KeyID: newKeyID}); err != nil {
		return err
	}
	if err := w.objects.RemoveWrap(ctx, q, objectID, oldKeyID); err != nil {
		return err
	}
	// 封装边切换成功：对象标记为已完成重新封装。
	if err := w.objects.UpdateStatus(ctx, q, objectID, model.ObjectRewrapped); err != nil {
		return err
	}
	return nil
}

// WrapsOfObject 返回对象的全部封装密钥。
func (w *WrapService) WrapsOfObject(ctx context.Context, objectID string) ([]*model.Key, error) {
	keyIDs, err := w.objects.WrapKeyIDs(ctx, w.st.DB(), objectID)
	if err != nil {
		return nil, err
	}
	var out []*model.Key
	for _, id := range keyIDs {
		k, err := w.keys.Get(ctx, w.st.DB(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// AllWraps 返回全部封装边（供证明指纹计算）。
func (w *WrapService) AllWraps(ctx context.Context) ([]*model.Wrap, error) {
	return w.objects.ListWraps(ctx, w.st.DB(), "", "")
}
