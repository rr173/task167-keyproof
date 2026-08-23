package httpapi

import (
	"net/http"

	"task167-keyproof/internal/model"
)

// ---- 证明 ----

type computeProofReq struct {
	PlanID string `json:"planId"`
	StepID string `json:"stepId,omitempty"`
}

func (s *Server) handleComputeProof(w http.ResponseWriter, r *http.Request) {
	var req computeProofReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.PlanID == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	p, err := s.app.ComputeProof(r.Context(), req.PlanID, req.StepID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListProofs(w http.ResponseWriter, r *http.Request) {
	planID := r.URL.Query().Get("planId")
	if planID != "" {
		proofs, err := s.app.ListProofs(r.Context(), planID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, proofs)
		return
	}
	proofs, err := s.app.ListAllProofs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proofs)
}

func (s *Server) handleGetProof(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	all, err := s.app.ListAllProofs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, p := range all {
		if p.ID == id {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeErr(w, model.ErrNotFound)
}

// ---- 执行记录 ----

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	planID := r.URL.Query().Get("planId")
	if planID != "" {
		execs, err := s.app.Runner.ListByPlan(r.Context(), planID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, execs)
		return
	}
	execs, err := s.app.Runner.ListAll(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, execs)
}

// ---- 统计 ----

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.app.ComputeStats(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
