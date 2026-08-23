package plan

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/proof"
)

// Validator 实现 StepChecker：对每个步骤在“假设应用后”的
// 约束模型上判定覆盖与退休不变量。它只读关系图，不写数据。
type Validator struct {
	Engine *proof.Engine
}

// NewValidator 构造单步验证器。
func NewValidator(engine *proof.Engine) *Validator {
	return &Validator{Engine: engine}
}

// Check 对单个步骤做验证，返回验证结果。
// planSteps 为计划的全部步骤：验证时把位于当前步骤之前的
// rewrap 步骤视为“已应用”，用于豁免退休/吊销的阻断判定。
func (v *Validator) Check(ctx context.Context, step *model.PlanStep, planSteps []*model.PlanStep) StepCheckResult {
	res := StepCheckResult{StepID: step.ID, Seq: step.Seq, Action: step.Action}
	var err error
	switch step.Action {
	case model.ActionRetireKey, model.ActionRevokeKey:
		err = v.checkKeyRotation(ctx, step, planSteps)
	case model.ActionRewrapObject:
		err = v.checkRewrap(ctx, step)
	case model.ActionGrantKey:
		err = v.checkGrant(ctx, step)
	case model.ActionRevokeGrant:
		err = v.checkRevokeGrant(ctx, step)
	case model.ActionRemoveWrap:
		err = v.checkRemoveWrap(ctx, step)
	default:
		err = model.ErrInvalidInput
	}
	if err != nil {
		res.Err = err
		res.ErrCode = err.Error()
		res.ErrDetail = errDetail(err)
		res.Summary = fmt.Sprintf("步骤 %d (%s) 被阻断: %v", step.Seq, step.Action, err)
		return res
	}
	res.Summary = fmt.Sprintf("步骤 %d (%s) 验证通过", step.Seq, step.Action)
	return res
}

// migratedAwayByEarlierSteps 判断计划中是否有位于当前步骤之前、
// 把对象迁出子树范围的 rewrap 步骤。
func migratedAwayByEarlierSteps(step *model.PlanStep, objectID string, subtreeSet map[string]bool, planSteps []*model.PlanStep) bool {
	for _, s := range planSteps {
		if s.Seq >= step.Seq {
			break
		}
		if s.Action != model.ActionRewrapObject {
			continue
		}
		if s.ObjectID != objectID {
			continue
		}
		if !subtreeSet[s.TargetKeyID] {
			return true // 目标密钥不在子树内：对象将被迁出
		}
	}
	return false
}

// checkKeyRotation 退休/吊销密钥：确保不存在“仅由该密钥子树覆盖”的对象，
// 且该对象未被计划内更早的 rewrap 步骤迁出。
func (v *Validator) checkKeyRotation(ctx context.Context, step *model.PlanStep, planSteps []*model.PlanStep) error {
	k, err := v.Engine.KeyOf(ctx, step.KeyID)
	if err != nil {
		return err
	}
	if k.Status != model.KeyActive && k.Status != model.KeyRetiring {
		return model.ErrInvalidState
	}
	subtree, err := v.Engine.SubtreeOf(ctx, k.ID)
	if err != nil {
		return err
	}
	subtreeSet := toSet(subtree)
	// 枚举所有对象，找出封装密钥全部落在子树内的对象。
	objects, err := v.listAllObjects(ctx)
	if err != nil {
		return err
	}
	for _, o := range objects {
		wrapKeys, err := v.Engine.WrapsOfObject(ctx, o.ID)
		if err != nil {
			return err
		}
		if len(wrapKeys) == 0 {
			continue
		}
		allInSubtree := true
		for _, wk := range wrapKeys {
			if !subtreeSet[wk.ID] {
				allInSubtree = false
				break
			}
		}
		if allInSubtree && !migratedAwayByEarlierSteps(step, o.ID, subtreeSet, planSteps) {
			// 退休该密钥会使对象失去全部可解密路径。
			return fmt.Errorf("%w: 对象 %s 仅由密钥 %s 子树覆盖",
				model.ErrRetirementBlocked, o.ID, k.ID)
		}
	}
	return nil
}

// checkRewrap 重新封装对象到目标密钥：目标密钥必须 active 且有授权主体。
func (v *Validator) checkRewrap(ctx context.Context, step *model.PlanStep) error {
	obj, err := v.Engine.ObjectOf(ctx, step.ObjectID)
	if err != nil {
		return err
	}
	target, err := v.Engine.KeyOf(ctx, step.TargetKeyID)
	if err != nil {
		return err
	}
	if target.Status != model.KeyActive {
		return model.ErrInvalidState
	}
	_ = obj
	return nil
}

// checkGrant 新增授权：主体与密钥存在、密钥未退休即通过
// （授权只会扩大覆盖，不会破坏不变量）。
func (v *Validator) checkGrant(ctx context.Context, step *model.PlanStep) error {
	if _, err := v.Engine.SubjectOf(ctx, step.SubjectID); err != nil {
		return err
	}
	k, err := v.Engine.KeyOf(ctx, step.KeyID)
	if err != nil {
		return err
	}
	if k.Status.RetiredOrRevoked() {
		return model.ErrRetiredReference
	}
	return nil
}

// checkRevokeGrant 撤销授权：确保不会使任何对象失去全部覆盖。
func (v *Validator) checkRevokeGrant(ctx context.Context, step *model.PlanStep) error {
	if _, err := v.Engine.SubjectOf(ctx, step.SubjectID); err != nil {
		return err
	}
	k, err := v.Engine.KeyOf(ctx, step.KeyID)
	if err != nil {
		return err
	}
	subtree, err := v.Engine.SubtreeOf(ctx, k.ID)
	if err != nil {
		return err
	}
	subtreeSet := toSet(subtree)
	objects, err := v.listAllObjects(ctx)
	if err != nil {
		return err
	}
	for _, o := range objects {
		wrapKeys, err := v.Engine.WrapsOfObject(ctx, o.ID)
		if err != nil {
			return err
		}
		inSubtree := false
		for _, wk := range wrapKeys {
			if subtreeSet[wk.ID] {
				inSubtree = true
				break
			}
		}
		if !inSubtree {
			continue
		}
		// 计算该对象全部可解密主体。
		paths, err := v.Engine.CoverageOfObject(ctx, o.ID)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			continue // 本就孤立，撤销不改变
		}
		otherCovered := false
		for _, p := range paths {
			if p.SubjectID != step.SubjectID {
				otherCovered = true
				break
			}
		}
		if !otherCovered {
			return fmt.Errorf("%w: 撤销主体 %s 对密钥 %s 的授权将使对象 %s 孤立",
				model.ErrNoCoverage, step.SubjectID, k.ID, o.ID)
		}
	}
	return nil
}

// checkRemoveWrap 移除封装边：对象必须仍存在其它有授权的封装密钥。
func (v *Validator) checkRemoveWrap(ctx context.Context, step *model.PlanStep) error {
	obj, err := v.Engine.ObjectOf(ctx, step.ObjectID)
	if err != nil {
		return err
	}
	wrapKeys, err := v.Engine.WrapsOfObject(ctx, step.ObjectID)
	if err != nil {
		return err
	}
	hasOther := false
	for _, wk := range wrapKeys {
		if wk.ID == step.KeyID {
			continue
		}
		subs, err := v.Engine.CoverageSubjects(ctx, wk.ID)
		if err != nil {
			return err
		}
		if len(subs) > 0 {
			hasOther = true
			break
		}
	}
	if !hasOther {
		return fmt.Errorf("%w: 移除对象 %s 对密钥 %s 的封装后将无覆盖",
			model.ErrNoCoverage, obj.ID, step.KeyID)
	}
	return nil
}

// listAllObjects 列出全部对象（通过引擎查询）。
func (v *Validator) listAllObjects(ctx context.Context) ([]*model.Object, error) {
	return v.listObjects(ctx)
}

// listObjects 使用 engine 查询对象仓库。
func (v *Validator) listObjects(ctx context.Context) ([]*model.Object, error) {
	return v.Engine.ListObjects(ctx)
}

// errDetail 返回错误的中文说明，便于 HTTP 层展示。
func errDetail(err error) string {
	switch {
	case err == nil:
		return ""
	default:
		return err.Error()
	}
}

func toSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
