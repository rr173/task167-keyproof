package httpapi

import (
	"net/http"

	"task167-keyproof/internal/model"
)

// ---- 轮换计划 ----

type createPlanReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var req createPlanReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	p, err := s.app.CreatePlan(r.Context(), req.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.app.ListPlans(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

// planDetail 返回计划详情（含步骤与证明快照）。
func (s *Server) planDetail(w http.ResponseWriter, r *http.Request, id string) {
	p, err := s.app.GetPlan(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	steps, err := s.app.ListPlanSteps(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	proofs, err := s.app.ListProofs(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":   p,
		"steps":  steps,
		"proofs": proofs,
	})
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	s.planDetail(w, r, pathParam(r, "id"))
}

type addStepReq struct {
	Action      model.StepAction `json:"action"`
	KeyID       string           `json:"keyId,omitempty"`
	ObjectID    string           `json:"objectId,omitempty"`
	SubjectID   string           `json:"subjectId,omitempty"`
	TargetKeyID string           `json:"targetKeyId,omitempty"`
}

func (s *Server) handleAddStep(w http.ResponseWriter, r *http.Request) {
	var req addStepReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	step, err := s.app.AddPlanStep(r.Context(), pathParam(r, "id"),
		req.Action, req.KeyID, req.ObjectID, req.SubjectID, req.TargetKeyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (s *Server) handleValidatePlan(w http.ResponseWriter, r *http.Request) {
	report, err := s.app.ValidatePlan(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK
	if report.Status == model.PlanBlocked {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, report)
}

func (s *Server) handleApprovePlan(w http.ResponseWriter, r *http.Request) {
	report, err := s.app.ApprovePlan(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK
	if !report.Approved {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, report)
}

type executeReq struct {
	Seq int `json:"seq"`
}

func (s *Server) handleExecuteStep(w http.ResponseWriter, r *http.Request) {
	var req executeReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Seq < 1 {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	res, err := s.app.ExecutePlanStep(r.Context(), pathParam(r, "id"), req.Seq)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	res, err := s.app.RollbackPlan(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	rep, err := s.app.RecoverPlan(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handlePlanGaps(w http.ResponseWriter, r *http.Request) {
	chains, err := s.app.PlanGaps(r.Context(), pathParam(r, "id"), r.URL.Query().Get("subjectId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chains)
}

func (s *Server) handlePlanResiduals(w http.ResponseWriter, r *http.Request) {
	residuals, err := s.app.PlanResiduals(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, residuals)
}
