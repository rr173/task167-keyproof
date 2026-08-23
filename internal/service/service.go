// Package service 是应用编排层：聚合 store、relation、proof、
// plan、execution 各模块，为 HTTP 层提供统一入口。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"task167-keyproof/internal/execution"
	"task167-keyproof/internal/model"
	"task167-keyproof/internal/plan"
	"task167-keyproof/internal/proof"
	"task167-keyproof/internal/relation"
	"task167-keyproof/internal/store"
)

// App 聚合全部领域能力。
type App struct {
	St        *store.Store
	Reg       *relation.Registry
	Engine    *proof.Engine
	Plans     *plan.Service
	Validator *plan.Validator
	Runner    *execution.Runner

	keys     *store.KeyRepo
	subjects *store.SubjectRepo
	objects  *store.ObjectRepo
	execs    *store.ExecutionRepo
}

// New 构造应用。
func New(st *store.Store) *App {
	reg := relation.NewRegistry(st)
	engine := proof.NewEngine(st, reg)
	return &App{
		St:        st,
		Reg:       reg,
		Engine:    engine,
		Plans:     plan.NewService(st),
		Validator: plan.NewValidator(engine),
		Runner:    execution.NewRunner(st, reg),
		keys:      store.NewKeyRepo(),
		subjects:  store.NewSubjectRepo(),
		objects:   store.NewObjectRepo(),
		execs:     store.NewExecutionRepo(),
	}
}

// NewID 生成短随机 ID，前缀区分实体类型。
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 理论不可达；退化为时间戳。
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// ---- 密钥 ----

// CreateKey 登记密钥（自动生成 ID）。
func (a *App) CreateKey(ctx context.Context, name string, kind model.KeyKind, parentID string) (*model.Key, error) {
	k := &model.Key{
		ID:       NewID("key"),
		Name:     name,
		Kind:     kind,
		Status:   model.KeyCandidate,
		ParentID: parentID,
	}
	if err := a.Reg.CreateKey(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// ListKeys 列出全部密钥。
func (a *App) ListKeys(ctx context.Context) ([]*model.Key, error) {
	return a.keys.List(ctx, a.St.DB())
}

// GetKey 查询密钥。
func (a *App) GetKey(ctx context.Context, id string) (*model.Key, error) {
	return a.keys.Get(ctx, a.St.DB(), id)
}

// ---- 主体 ----

// CreateSubject 登记主体。
func (a *App) CreateSubject(ctx context.Context, name string) (*model.Subject, error) {
	s := &model.Subject{ID: NewID("sub"), Name: name, Status: model.SubjectActive}
	if err := a.Reg.CreateSubject(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// ListSubjects 列出全部主体。
func (a *App) ListSubjects(ctx context.Context) ([]*model.Subject, error) {
	return a.subjects.List(ctx, a.St.DB())
}

// GetSubject 查询主体。
func (a *App) GetSubject(ctx context.Context, id string) (*model.Subject, error) {
	return a.subjects.Get(ctx, a.St.DB(), id)
}

// AddGrant 授权主体到密钥。
func (a *App) AddGrant(ctx context.Context, subjectID, keyID string) error {
	return relation.GrantServiceOf(a.Reg).AddGrant(ctx, subjectID, keyID)
}

// RevokeGrant 撤销授权。
func (a *App) RevokeGrant(ctx context.Context, subjectID, keyID string) error {
	return relation.GrantServiceOf(a.Reg).RevokeGrant(ctx, subjectID, keyID)
}

// GrantsOfSubject 返回主体授权的密钥。
func (a *App) GrantsOfSubject(ctx context.Context, subjectID string) ([]*model.Key, error) {
	return relation.GrantServiceOf(a.Reg).GrantsOfSubject(ctx, subjectID)
}

// ---- 对象 ----

// CreateObject 登记加密对象。
func (a *App) CreateObject(ctx context.Context, name string) (*model.Object, error) {
	o := &model.Object{ID: NewID("obj"), Name: name, Status: model.ObjectProtected}
	if err := a.Reg.CreateObject(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// ListObjects 列出全部对象。
func (a *App) ListObjects(ctx context.Context) ([]*model.Object, error) {
	return a.objects.List(ctx, a.St.DB())
}

// GetObject 查询对象。
func (a *App) GetObject(ctx context.Context, id string) (*model.Object, error) {
	return a.objects.Get(ctx, a.St.DB(), id)
}

// Wrap 绑定对象封装边。
func (a *App) Wrap(ctx context.Context, objectID, keyID string) error {
	return relation.WrapServiceOf(a.Reg).AddWrap(ctx, objectID, keyID)
}

// Rewrap 重新封装对象。
func (a *App) Rewrap(ctx context.Context, objectID, oldKeyID, newKeyID string) error {
	return relation.WrapServiceOf(a.Reg).Rewrap(ctx, objectID, oldKeyID, newKeyID)
}

// ObjectCoverage 返回对象的覆盖路径。
func (a *App) ObjectCoverage(ctx context.Context, objectID string) ([]*proof.CoveragePath, error) {
	return a.Engine.CoverageOfObject(ctx, objectID)
}

// Stats 汇总统计。
type Stats struct {
	Keys        map[string]int `json:"keys"`
	Objects     map[string]int `json:"objects"`
	Plans       map[string]int `json:"plans"`
	Subjects    int            `json:"subjects"`
	Grants      int            `json:"grants"`
	Wraps       int            `json:"wraps"`
	Proofs      int            `json:"proofs"`
	Executions  int            `json:"executions"`
	Residuals   int            `json:"residuals"`
	RootKeysNoAuth int         `json:"rootKeysWithoutAuth"`
}

// ComputeStats 计算统计信息。
func (a *App) ComputeStats(ctx context.Context) (*Stats, error) {
	keys, err := a.keys.List(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	objects, err := a.objects.List(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	plans, err := a.Plans.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	grants, err := a.subjects.GrantCount(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	wraps, err := a.objects.WrapCount(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	subjects, err := a.subjects.List(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	execs, err := a.execs.ListAll(ctx, a.St.DB())
	if err != nil {
		return nil, err
	}
	proofs, err := a.Engine.ListAllProofs(ctx)
	if err != nil {
		return nil, err
	}
	residuals, err := a.Engine.ListResiduals(ctx)
	if err != nil {
		return nil, err
	}
	rootNoAuth, err := a.Engine.RootKeysWithoutAuth(ctx)
	if err != nil {
		return nil, err
	}

	st := &Stats{
		Keys:            countByStatus(keys, func(k *model.Key) string { return string(k.Status) }),
		Objects:         countByStatus(objects, func(o *model.Object) string { return string(o.Status) }),
		Plans:           countByStatus(plans, func(p *model.Plan) string { return string(p.Status) }),
		Subjects:        len(subjects),
		Grants:          grants,
		Wraps:           wraps,
		Proofs:          len(proofs),
		Executions:      len(execs),
		Residuals:       len(residuals),
		RootKeysNoAuth:  len(rootNoAuth),
	}
	return st, nil
}

func countByStatus[T any](items []T, f func(T) string) map[string]int {
	m := map[string]int{}
	for _, it := range items {
		m[f(it)]++
	}
	return m
}
