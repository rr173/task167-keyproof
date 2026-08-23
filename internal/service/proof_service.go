package service

import (
	"context"
	"fmt"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/proof"
)

// ---- 证明 ----

// ComputeProof 为计划某步骤计算覆盖证明快照。
// 证明判定：
//   - 存在孤立对象（无任何可解密主体） -> gap；
//   - 存在已退休/已吊销密钥的残留引用 -> residual；
//   - 二者皆无 -> sufficient。
//
// 快照绑定输入边指纹与摘要哈希，重启后可通过指纹校验防篡改。
func (a *App) ComputeProof(ctx context.Context, planID, stepID string) (*model.Proof, error) {
	steps, err := a.Plans.ListSteps(ctx, planID)
	if err != nil {
		return nil, err
	}
	if stepID == "" && len(steps) > 0 {
		// 未指定步骤时默认计算最后一个步骤的证明。
		stepID = steps[len(steps)-1].ID
	}
	var target *model.PlanStep
	for _, s := range steps {
		if s.ID == stepID {
			target = s
			break
		}
	}
	if target == nil {
		return nil, model.ErrNotFound
	}

	orphans, err := a.orphanedObjects(ctx)
	if err != nil {
		return nil, err
	}
	residuals, err := a.Engine.ListResiduals(ctx)
	if err != nil {
		return nil, err
	}
	inputFP, err := a.Engine.Fingerprint(ctx)
	if err != nil {
		return nil, err
	}

	status := model.ProofSufficient
	summary := fmt.Sprintf("计划 %s 步骤 %d 覆盖充分且无退休残留", planID, target.Seq)
	if len(orphans) > 0 {
		status = model.ProofGap
		summary = fmt.Sprintf("存在 %d 个孤立对象（无授权主体可解密）", len(orphans))
	} else if len(residuals) > 0 {
		status = model.ProofResidual
		summary = fmt.Sprintf("存在 %d 条退休残留引用", len(residuals))
	}

	p := &model.Proof{
		ID:      NewID("proof"),
		PlanID:  planID,
		StepID:  stepID,
		Status:  status,
		Digest:  proof.DigestOf(status, inputFP, summary),
		InputFP: inputFP,
		Summary: summary,
	}
	if err := a.Engine.SaveProof(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// orphanedObjects 返回全部孤立对象。
func (a *App) orphanedObjects(ctx context.Context) ([]*model.Object, error) {
	objects, err := a.ListObjects(ctx)
	if err != nil {
		return nil, err
	}
	var out []*model.Object
	for _, o := range objects {
		ok, err := a.Engine.IsOrphaned(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, o)
		}
	}
	return out, nil
}

// PlanGaps 计算计划中的覆盖缺口证据链。
// 若指定 subjectID，对每个孤立对象给出该主体到对象的最短缺口链；
// 否则输出对象自身缺口摘要。
func (a *App) PlanGaps(ctx context.Context, planID, subjectID string) ([]*model.EvidenceChain, error) {
	if _, err := a.GetPlan(ctx, planID); err != nil {
		return nil, err
	}
	orphans, err := a.orphanedObjects(ctx)
	if err != nil {
		return nil, err
	}
	var chains []*model.EvidenceChain
	if subjectID != "" {
		if _, err := a.GetSubject(ctx, subjectID); err != nil {
			return nil, err
		}
		for _, o := range orphans {
			chain, err := a.Engine.ShortestPath(ctx, subjectID, o.ID)
			if err != nil {
				return nil, err
			}
			if chain != nil {
				chains = append(chains, chain)
			}
		}
		return chains, nil
	}
	// 无主体参数：输出对象级缺口摘要。
	for _, o := range orphans {
		wrapKeys, err := a.Engine.WrapsOfObject(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		nodes := []model.ChainNode{{Role: "object", Name: o.Name, ID: o.ID}}
		for _, wk := range wrapKeys {
			nodes = append(nodes, model.ChainNode{Role: "key", Name: wk.Name, ID: wk.ID})
		}
		nodes = append(nodes, model.ChainNode{Role: "gap", Name: "无授权主体", ID: ""})
		chains = append(chains, &model.EvidenceChain{
			Kind:    "gap",
			Length:  len(nodes) - 1,
			Nodes:   nodes,
			Summary: fmt.Sprintf("对象 %s 为孤立对象：无任何主体可解密", o.Name),
		})
	}
	return chains, nil
}

// PlanResiduals 返回计划相关的退休残留（全局扫描）。
func (a *App) PlanResiduals(ctx context.Context, planID string) ([]*proof.Residual, error) {
	if _, err := a.GetPlan(ctx, planID); err != nil {
		return nil, err
	}
	return a.Engine.ListResiduals(ctx)
}

// ListProofs 列出计划证明快照。
func (a *App) ListProofs(ctx context.Context, planID string) ([]*model.Proof, error) {
	return a.Engine.ListProofs(ctx, planID)
}

// ListAllProofs 列出全部证明快照。
func (a *App) ListAllProofs(ctx context.Context) ([]*model.Proof, error) {
	return a.Engine.ListAllProofs(ctx)
}
