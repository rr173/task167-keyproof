package httpapi

import (
	"net/http"

	"task167-keyproof/internal/model"
)

// ---- 对象与封装 ----

type createObjectReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateObject(w http.ResponseWriter, r *http.Request) {
	var req createObjectReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	o, err := s.app.CreateObject(r.Context(), req.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	objs, err := s.app.ListObjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objs)
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	o, err := s.app.GetObject(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	wrapKeys, err := s.app.Engine.WrapsOfObject(r.Context(), o.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": o,
		"wraps":  wrapKeys,
	})
}

type wrapReq struct {
	KeyID string `json:"keyId"`
}

func (s *Server) handleWrapObject(w http.ResponseWriter, r *http.Request) {
	var req wrapReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.KeyID == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	if err := s.app.Wrap(r.Context(), pathParam(r, "id"), req.KeyID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"objectId": pathParam(r, "id"), "keyId": req.KeyID})
}

type rewrapReq struct {
	OldKeyID string `json:"oldKeyId"`
	NewKeyID string `json:"newKeyId"`
}

func (s *Server) handleRewrapObject(w http.ResponseWriter, r *http.Request) {
	var req rewrapReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.OldKeyID == "" || req.NewKeyID == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	if err := s.app.Rewrap(r.Context(), pathParam(r, "id"), req.OldKeyID, req.NewKeyID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"objectId": pathParam(r, "id"),
		"oldKeyId": req.OldKeyID,
		"newKeyId": req.NewKeyID,
	})
}

func (s *Server) handleObjectCoverage(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if _, err := s.app.GetObject(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	paths, err := s.app.ObjectCoverage(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	orphaned, err := s.app.Engine.IsOrphaned(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"objectId": id,
		"orphaned": orphaned,
		"coverage": paths,
		"count":    len(paths),
	})
}
