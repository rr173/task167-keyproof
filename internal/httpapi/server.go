// Package httpapi 提供 HTTP 接口层，路由统一以 /api 前缀暴露。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"task167-keyproof/internal/model"
	"task167-keyproof/internal/service"
)

// Server 是 HTTP 服务。
type Server struct {
	app *service.App
	mux *http.ServeMux
}

// New 构造 HTTP 服务并注册全部路由。
func New(app *service.App) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler 返回路由处理器。
func (s *Server) Handler() http.Handler { return s.mux }

// routes 注册全部 API。
func (s *Server) routes() {
	// 密钥
	s.mux.HandleFunc("POST /api/keys", s.handleCreateKey)
	s.mux.HandleFunc("GET /api/keys", s.handleListKeys)
	s.mux.HandleFunc("GET /api/keys/{id}", s.handleGetKey)
	s.mux.HandleFunc("POST /api/keys/{id}/activate", s.handleActivateKey)
	s.mux.HandleFunc("POST /api/keys/{id}/retiring", s.handleMarkRetiring)
	s.mux.HandleFunc("POST /api/keys/{id}/retire", s.handleRetireKey)
	s.mux.HandleFunc("POST /api/keys/{id}/revoke", s.handleRevokeKey)
	s.mux.HandleFunc("PUT /api/keys/{id}/parent", s.handleMoveKey)

	// 主体与授权
	s.mux.HandleFunc("POST /api/subjects", s.handleCreateSubject)
	s.mux.HandleFunc("GET /api/subjects", s.handleListSubjects)
	s.mux.HandleFunc("GET /api/subjects/{id}", s.handleGetSubject)
	s.mux.HandleFunc("POST /api/subjects/{id}/grants", s.handleAddGrant)
	s.mux.HandleFunc("DELETE /api/subjects/{id}/grants/{keyId}", s.handleRevokeGrant)

	// 对象与封装
	s.mux.HandleFunc("POST /api/objects", s.handleCreateObject)
	s.mux.HandleFunc("GET /api/objects", s.handleListObjects)
	s.mux.HandleFunc("GET /api/objects/{id}", s.handleGetObject)
	s.mux.HandleFunc("POST /api/objects/{id}/wrap", s.handleWrapObject)
	s.mux.HandleFunc("POST /api/objects/{id}/rewrap", s.handleRewrapObject)
	s.mux.HandleFunc("GET /api/objects/{id}/coverage", s.handleObjectCoverage)

	// 计划
	s.mux.HandleFunc("POST /api/plans", s.handleCreatePlan)
	s.mux.HandleFunc("GET /api/plans", s.handleListPlans)
	s.mux.HandleFunc("GET /api/plans/{id}", s.handleGetPlan)
	s.mux.HandleFunc("POST /api/plans/{id}/steps", s.handleAddStep)
	s.mux.HandleFunc("POST /api/plans/{id}/validate", s.handleValidatePlan)
	s.mux.HandleFunc("POST /api/plans/{id}/approve", s.handleApprovePlan)
	s.mux.HandleFunc("POST /api/plans/{id}/execute", s.handleExecuteStep)
	s.mux.HandleFunc("POST /api/plans/{id}/rollback", s.handleRollback)
	s.mux.HandleFunc("GET /api/plans/{id}/recovery", s.handleRecovery)
	s.mux.HandleFunc("GET /api/plans/{id}/gaps", s.handlePlanGaps)
	s.mux.HandleFunc("GET /api/plans/{id}/residuals", s.handlePlanResiduals)

	// 证明
	s.mux.HandleFunc("POST /api/proofs/compute", s.handleComputeProof)
	s.mux.HandleFunc("GET /api/proofs", s.handleListProofs)
	s.mux.HandleFunc("GET /api/proofs/{id}", s.handleGetProof)

	// 执行与统计
	s.mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
}

// errorStatus 将领域错误映射为 HTTP 状态码。
func errorStatus(err error) int {
	switch {
	case errors.Is(err, model.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrRetiredReference),
		errors.Is(err, model.ErrCrossPlan):
		return http.StatusConflict
	case errors.Is(err, model.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, model.ErrInvalidState), errors.Is(err, model.ErrNotDrafted),
		errors.Is(err, model.ErrPlanNotExecutable), errors.Is(err, model.ErrRollbackForbidden),
		errors.Is(err, model.ErrImmutable):
		return http.StatusUnprocessableEntity
	case errors.Is(err, model.ErrCycleDetected), errors.Is(err, model.ErrNoRootAuth),
		errors.Is(err, model.ErrNoCoverage), errors.Is(err, model.ErrRetirementBlocked),
		errors.Is(err, model.ErrEmptyPlan):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

// writeErr 输出错误响应。
func writeErr(w http.ResponseWriter, err error) {
	status := errorStatus(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		msg = "internal error"
		log.Printf("internal error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseBody 解析请求体 JSON 到目标结构。
func parseBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("请求体解析失败: %w", model.ErrInvalidInput)
	}
	return nil
}

// pathParam 获取路径参数。
func pathParam(r *http.Request, name string) string {
	return r.PathValue(name)
}
