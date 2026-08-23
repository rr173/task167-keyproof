package execution_test

import (
	"context"
	"errors"
	"testing"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

// TestExecuteStepRetriesAfterApplyFailure 证明失败路径不遗留目标锁：
// 当 applyStep 因外部状态冲突而失败时，本次获取的跨计划锁必须被释放，
// 这样修复外部状态后再次执行同一步骤能够正常重试，而不是收到 ErrCrossPlan。
func TestExecuteStepRetriesAfterApplyFailure(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := service.New(st)

	root, err := app.CreateKey(ctx, "root", model.KindRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Reg.ActivateKey(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	subj, err := app.CreateSubject(ctx, "subject")
	if err != nil {
		t.Fatal(err)
	}
	// 预置已存在的授权：grant_key 步骤在 applyStep 时会因
	// 授权边已存在（UNIQUE 冲突 -> ErrConflict）而失败，但单步验证
	// 不会拒绝（checkGrant 仅校验主体存在、密钥未退休）。
	if err := app.AddGrant(ctx, subj.ID, root.ID); err != nil {
		t.Fatal(err)
	}

	plan, err := app.CreatePlan(ctx, "grant subject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddPlanStep(ctx, plan.ID, model.ActionGrantKey, root.ID, "", subj.ID, ""); err != nil {
		t.Fatal(err)
	}
	if report, err := app.ValidatePlan(ctx, plan.ID); err != nil || report.Status != model.PlanExecutable {
		t.Fatalf("validate report = %#v, err = %v", report, err)
	}
	if report, err := app.ApprovePlan(ctx, plan.ID); err != nil || !report.Approved {
		t.Fatalf("approve report = %#v, err = %v", report, err)
	}

	// 第一次执行：applyStep 因授权已存在而失败。
	if _, err := app.ExecutePlanStep(ctx, plan.ID, 1); err == nil {
		t.Fatalf("首次执行应因授权冲突而失败")
	}

	// 失败路径不得遗留目标锁：直接查询 plan_locks 应为空。
	execs := store.NewExecutionRepo()
	locks, err := execs.LocksOfPlan(ctx, app.St.DB(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 0 {
		t.Fatalf("失败路径遗留了目标锁: %#v", locks)
	}

	// 修复外部状态：撤销预置授权，使再次 applyStep 可以成功插入。
	if err := app.RevokeGrant(ctx, subj.ID, root.ID); err != nil {
		t.Fatal(err)
	}

	// 再次执行同一步骤：必须能正常重试，而不是 ErrCrossPlan。
	res, err := app.ExecutePlanStep(ctx, plan.ID, 1)
	if err != nil {
		// 将错误与 ErrCrossPlan 区分开，便于定位泄漏锁的回归。
		if errors.Is(err, model.ErrCrossPlan) {
			t.Fatalf("重试收到跨计划占用 ErrCrossPlan，说明失败路径遗留了目标锁: %v", err)
		}
		t.Fatalf("重试应成功执行，但收到错误: %v", err)
	}
	if res.Status != string(model.ExecApplied) || res.AppliedCount != 1 {
		t.Fatalf("重试结果 = %#v; 期望 status=applied appliedCount=1", res)
	}
}
