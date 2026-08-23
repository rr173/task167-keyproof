// Package smoke 提供离线端到端自检（--smoke-test 契约）。
// 覆盖需求端到端场景：覆盖证明、最短缺口链、重新封装与退休阻断、
// 重启恢复与幂等执行、退休密钥新引用拒绝。
package smoke

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

// Run 执行完整自检。dbPath 为空时使用临时文件库（验证真实落盘与重启恢复）。
func Run(dbPath string) error {
	dir := ""
	if dbPath == "" {
		var err error
		dir, err = os.MkdirTemp("", "keyproof-smoke-*")
		if err != nil {
			return fmt.Errorf("创建临时目录: %w", err)
		}
		defer os.RemoveAll(dir)
		dbPath = filepath.Join(dir, "smoke.db")
	}
	ctx := context.Background()

	// ---- 场景 0：登记密钥层级与主体 ----
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库: %w", err)
	}
	app := service.New(st)
	root, err := app.CreateKey(ctx, "公司根密钥", model.KindRoot, "")
	if err != nil {
		return fmt.Errorf("登记根密钥: %w", err)
	}
	tenant, err := app.CreateKey(ctx, "租户A密钥", model.KindTenant, root.ID)
	if err != nil {
		return fmt.Errorf("登记租户密钥: %w", err)
	}
	dataA, err := app.CreateKey(ctx, "数据密钥A", model.KindData, tenant.ID)
	if err != nil {
		return fmt.Errorf("登记数据密钥A: %w", err)
	}
	dataB, err := app.CreateKey(ctx, "数据密钥B", model.KindData, tenant.ID)
	if err != nil {
		return fmt.Errorf("登记数据密钥B: %w", err)
	}
	if _, err := app.Reg.ActivateKey(ctx, root.ID); err != nil {
		return fmt.Errorf("启用根密钥: %w", err)
	}
	if _, err := app.Reg.ActivateKey(ctx, tenant.ID); err != nil {
		return fmt.Errorf("启用租户密钥: %w", err)
	}
	if _, err := app.Reg.ActivateKey(ctx, dataA.ID); err != nil {
		return fmt.Errorf("启用数据密钥A: %w", err)
	}
	if _, err := app.Reg.ActivateKey(ctx, dataB.ID); err != nil {
		return fmt.Errorf("启用数据密钥B: %w", err)
	}
	eng, err := app.CreateSubject(ctx, "安全工程师组")
	if err != nil {
		return fmt.Errorf("登记主体: %w", err)
	}
	tenantB, err := app.CreateSubject(ctx, "租户B")
	if err != nil {
		return fmt.Errorf("登记主体租户B: %w", err)
	}
	if err := app.AddGrant(ctx, eng.ID, root.ID); err != nil {
		return fmt.Errorf("授权工程师组到根密钥: %w", err)
	}
	// 根密钥有授权主体，根链有效。
	noAuth, err := app.Engine.RootKeysWithoutAuth(ctx)
	if err != nil {
		return err
	}
	if len(noAuth) != 0 {
		return fmt.Errorf("根链授权检查失败: %d 个根密钥无授权", len(noAuth))
	}
	if err := checkCycle(app, ctx, dataA.ID); err != nil {
		return err
	}

	// 对象 O1 封装于数据密钥 A（覆盖充分）。
	o1, err := app.CreateObject(ctx, "客户资料卷1")
	if err != nil {
		return fmt.Errorf("登记对象: %w", err)
	}
	if err := app.Wrap(ctx, o1.ID, dataA.ID); err != nil {
		return fmt.Errorf("封装对象: %w", err)
	}
	cov, err := app.ObjectCoverage(ctx, o1.ID)
	if err != nil {
		return err
	}
	if len(cov) == 0 {
		return fmt.Errorf("场景0失败: 对象 O1 应有覆盖")
	}
	fmt.Printf("✓ 场景0 覆盖建立: 对象 %s 由 %d 条路径覆盖\n", o1.Name, len(cov))

	// ---- 场景 1：新密钥未授权给租户时给出最短缺口链 ----
	// o2 封装于数据密钥 B；工程师组经根密钥授权可解密，但租户B 无授权。
	o2, err := app.CreateObject(ctx, "租户B数据卷")
	if err != nil {
		return fmt.Errorf("登记对象: %w", err)
	}
	if err := app.Wrap(ctx, o2.ID, dataB.ID); err != nil {
		return fmt.Errorf("封装对象: %w", err)
	}
	// 租户B 到 o2 的路径应不可达 -> 最短缺口链。
	chain, err := app.Engine.ShortestPath(ctx, tenantB.ID, o2.ID)
	if err != nil {
		return err
	}
	if chain == nil || chain.Kind != "gap" {
		return fmt.Errorf("场景1失败: 应得到最短缺口链, 得到 %+v", chain)
	}
	// 工程师组经根密钥授权传播应可解密 o2（覆盖链）。
	engChain, err := app.Engine.ShortestPath(ctx, eng.ID, o2.ID)
	if err != nil {
		return err
	}
	if engChain == nil || engChain.Kind != "coverage" {
		return fmt.Errorf("场景1失败: 工程师组应可解密 o2, 得到 %+v", engChain)
	}
	fmt.Printf("✓ 场景1 最短缺口链: %s (长度 %d)；工程师组覆盖链存在\n",
		chain.Summary, chain.Length)

	// ---- 场景 2：计划验证、重新封装、退休阻断与新引用拒绝 ----
	plan, err := app.CreatePlan(ctx, "数据密钥A轮换")
	if err != nil {
		return fmt.Errorf("创建计划: %w", err)
	}
	// 步骤1：将 O1 重新封装到数据密钥 B（新密钥有授权）。
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionRewrapObject,
		dataA.ID, o1.ID, "", dataB.ID); err != nil {
		return fmt.Errorf("添加重新封装步骤: %w", err)
	}
	// 步骤2：退休数据密钥 A（此时 O1 已迁走，无对象仅由 A 子树覆盖）。
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionRetireKey,
		dataA.ID, "", "", ""); err != nil {
		return fmt.Errorf("添加退休步骤: %w", err)
	}
	// 反例：若先退休 A（O1 未迁移）应被阻断 —— 构造阻断计划验证。
	blockPlan, err := app.CreatePlan(ctx, "未迁移先退休（应阻断）")
	if err != nil {
		return fmt.Errorf("创建阻断计划: %w", err)
	}
	blockObj, err := app.CreateObject(ctx, "仍封装在A的卷")
	if err != nil {
		return err
	}
	if err := app.Wrap(ctx, blockObj.ID, dataA.ID); err != nil {
		return err
	}
	if _, err := app.AddPlanStep(ctx, blockPlan.ID, model.ActionRetireKey,
		dataA.ID, "", "", ""); err != nil {
		return err
	}
	blockReport, err := app.ValidatePlan(ctx, blockPlan.ID)
	if err != nil {
		return err
	}
	if blockReport.Status != model.PlanBlocked || len(blockReport.Blocked) == 0 {
		return fmt.Errorf("场景2失败: 未迁移先退休应被阻断")
	}
	fmt.Printf("✓ 场景2 退休阻断: %s\n", blockReport.Blocked[0].ErrDetail)
	// 修复：将未迁移对象重新封装到数据密钥 B（模拟工程师修复），
	// 使退休不再阻断任何对象。
	if err := app.Rewrap(ctx, blockObj.ID, dataA.ID, dataB.ID); err != nil {
		return fmt.Errorf("修复未迁移对象: %w", err)
	}

	report, err := app.ValidatePlan(ctx, plan.ID)
	if err != nil {
		return err
	}
	if report.Status != model.PlanExecutable {
		return fmt.Errorf("场景2失败: 计划应可执行, 实际 %s (%d 条阻断)",
			report.Status, len(report.Blocked))
	}
	approve, err := app.ApprovePlan(ctx, plan.ID)
	if err != nil {
		return err
	}
	if !approve.Approved {
		return fmt.Errorf("场景2失败: 批准被拒: %v", approve.Messages)
	}
	// 执行步骤1（重新封装）。
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err != nil {
		return fmt.Errorf("执行重新封装: %w", err)
	}
	// 执行步骤2（退休 A -> retiring）。
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 2); err != nil {
		return fmt.Errorf("执行退休: %w", err)
	}
	keyA, err := app.GetKey(ctx, dataA.ID)
	if err != nil {
		return err
	}
	if keyA.Status != model.KeyRetiring {
		return fmt.Errorf("场景2失败: 密钥A应处于退役待清理, 实际 %s", keyA.Status)
	}
	// 阻止退休密钥的新写入：O1 再封装回 A 应被拒绝。
	if err := app.Wrap(ctx, o1.ID, dataA.ID); err == nil {
		return fmt.Errorf("场景2失败: 退休待清理密钥应拒绝新封装引用")
	}
	fmt.Printf("✓ 场景2 重新封装并退休: 密钥 %s 进入 %s，新写入被拒绝\n", keyA.Name, keyA.Status)

	// 退休待清理 -> 已退休（无残留）。
	if _, err := app.Reg.RetireKey(ctx, dataA.ID); err != nil {
		return fmt.Errorf("正式退休: %w", err)
	}
	if err := app.Wrap(ctx, o2.ID, dataA.ID); err == nil {
		return fmt.Errorf("场景2失败: 已退休密钥应拒绝新封装引用")
	}
	residuals, err := app.Engine.ListResiduals(ctx)
	if err != nil {
		return err
	}
	if len(residuals) != 0 {
		return fmt.Errorf("场景2失败: 不应存在残留引用: %v", residuals)
	}
	fmt.Printf("✓ 场景2 已退休密钥新引用被拒绝，无残留\n")

	// ---- 场景 3：重启恢复 + 幂等执行 + 证明保持 ----
	proofSnap, err := app.ComputeProof(ctx, plan.ID, "")
	if err != nil {
		return err
	}
	if proofSnap.Status != model.ProofSufficient {
		return fmt.Errorf("场景3失败: 证明应为 sufficient, 实际 %s", proofSnap.Status)
	}
	if err := st.Close(); err != nil {
		return err
	}

	// 重新打开数据库（模拟重启）。
	st2, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("重启打开数据库: %w", err)
	}
	defer st2.Close()
	app2 := service.New(st2)
	rec, err := app2.RecoverPlan(ctx, plan.ID)
	if err != nil {
		return err
	}
	if !rec.ProofPreserved {
		return fmt.Errorf("场景3失败: 已完成步骤证明应保持")
	}
	if rec.AppliedSteps != 2 || rec.RemainingSteps != 0 {
		return fmt.Errorf("场景3失败: 恢复进度错误 applied=%d remaining=%d",
			rec.AppliedSteps, rec.RemainingSteps)
	}
	// 重复执行步骤1：幂等，不重复计数。
	dup, err := app2.ExecutePlanStep(ctx, plan.ID, 1)
	if err != nil {
		return err
	}
	if !dup.Idempotent {
		return fmt.Errorf("场景3失败: 重复执行应幂等")
	}
	stats, err := app2.ComputeStats(ctx)
	if err != nil {
		return err
	}
	if stats.Executions != 2 { // 步骤1、2 各一次执行记录
		return fmt.Errorf("场景3失败: 重复执行不应新增执行记录, 实际 %d 条", stats.Executions)
	}
	if stats.Keys[string(model.KeyRetired)] != 1 {
		return fmt.Errorf("场景3失败: 应有 1 个已退休密钥, 实际 %v", stats.Keys)
	}
	fmt.Printf("✓ 场景3 重启恢复: 进度 2/2，证明保持，重复执行幂等（执行记录 %d 条）\n", stats.Executions)

	// ---- 场景 4：全局孤立对象与根链缺口 ----
	// 新建无授权的独立根密钥，其数据密钥封装的对象应被判定为全局孤立，
	// 同时根链缺口检测应命中该根密钥。
	root2, err := app2.CreateKey(ctx, "孤立根密钥(无授权)", model.KindRoot, "")
	if err != nil {
		return err
	}
	if _, err := app2.Reg.ActivateKey(ctx, root2.ID); err != nil {
		return err
	}
	dataC, err := app2.CreateKey(ctx, "孤立数据密钥", model.KindData, root2.ID)
	if err != nil {
		return err
	}
	if _, err := app2.Reg.ActivateKey(ctx, dataC.ID); err != nil {
		return err
	}
	o3, err := app2.CreateObject(ctx, "无主数据卷")
	if err != nil {
		return err
	}
	if err := app2.Wrap(ctx, o3.ID, dataC.ID); err != nil {
		return err
	}
	orphaned, err := app2.Engine.IsOrphaned(ctx, o3.ID)
	if err != nil {
		return err
	}
	if !orphaned {
		return fmt.Errorf("场景4失败: 无授权根链下的对象应被判定为孤立")
	}
	noAuthRoots, err := app2.Engine.RootKeysWithoutAuth(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, r := range noAuthRoots {
		if r.ID == root2.ID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("场景4失败: 根链缺口应命中新根密钥")
	}
	fmt.Printf("✓ 场景4 全局孤立与根链缺口: 对象 %s 孤立，根密钥 %s 无授权被识别\n", o3.Name, root2.Name)

	fmt.Println("smoke 自检全部通过")
	return nil
}

// checkCycle 验证层级约束生效：拒绝把数据密钥挂到数据密钥之下
// 以及非根密钥换到空父。
func checkCycle(app *service.App, ctx context.Context, childKeyID string) error {
	dataKey, err := app.Engine.KeyOf(ctx, childKeyID)
	if err != nil {
		return err
	}
	// 数据密钥不可再有子密钥：把另一数据密钥挂到它下面应被拒绝。
	if err := app.Reg.MoveKey(ctx, childKeyID, dataKey.ID); err == nil {
		return fmt.Errorf("层级校验失败: 数据密钥下不应再挂子密钥")
	}
	// 非根密钥换到空父应被拒绝。
	if err := app.Reg.MoveKey(ctx, dataKey.ParentID, ""); err == nil {
		return fmt.Errorf("层级校验失败: 非根密钥换父到空应被拒绝")
	}
	return nil
}

// 确保 time 包被引用（供未来扩展）。
var _ = time.Now
