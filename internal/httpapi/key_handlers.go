package httpapi

import (
	"net/http"

	"task167-keyproof/internal/model"
)

// ---- 密钥 ----

type createKeyReq struct {
	Name     string        `json:"name"`
	Kind     model.KeyKind `json:"kind"`
	ParentID string        `json:"parentId,omitempty"`
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var req createKeyReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name == "" || req.Kind == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	k, err := s.app.CreateKey(r.Context(), req.Name, req.Kind, req.ParentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.app.ListKeys(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.app.GetKey(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleActivateKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.app.Reg.ActivateKey(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleMarkRetiring(w http.ResponseWriter, r *http.Request) {
	k, err := s.app.Reg.MarkRetiring(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleRetireKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.app.Reg.RetireKey(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.app.Reg.RevokeKey(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

type moveKeyReq struct {
	ParentID string `json:"parentId"`
}

func (s *Server) handleMoveKey(w http.ResponseWriter, r *http.Request) {
	var req moveKeyReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.app.Reg.MoveKey(r.Context(), pathParam(r, "id"), req.ParentID); err != nil {
		writeErr(w, err)
		return
	}
	k, err := s.app.GetKey(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// ---- 主体与授权 ----

type createSubjectReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateSubject(w http.ResponseWriter, r *http.Request) {
	var req createSubjectReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	sub, err := s.app.CreateSubject(r.Context(), req.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) handleListSubjects(w http.ResponseWriter, r *http.Request) {
	subs, err := s.app.ListSubjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

func (s *Server) handleGetSubject(w http.ResponseWriter, r *http.Request) {
	sub, err := s.app.GetSubject(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	grants, err := s.app.GrantsOfSubject(r.Context(), sub.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject": sub,
		"grants":  grants,
	})
}

type grantReq struct {
	KeyID string `json:"keyId"`
}

func (s *Server) handleAddGrant(w http.ResponseWriter, r *http.Request) {
	var req grantReq
	if err := parseBody(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.KeyID == "" {
		writeErr(w, model.ErrInvalidInput)
		return
	}
	if err := s.app.AddGrant(r.Context(), pathParam(r, "id"), req.KeyID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"subjectId": pathParam(r, "id"), "keyId": req.KeyID})
}

func (s *Server) handleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	if err := s.app.RevokeGrant(r.Context(), pathParam(r, "id"), pathParam(r, "keyId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"revoked": "true"})
}

// handleRemoveSubject 软删除主体：移除后主体不再计入覆盖查询与
// 最短证据链，亦不再被判为可解密对象。
func (s *Server) handleRemoveSubject(w http.ResponseWriter, r *http.Request) {
	sub, err := s.app.RemoveSubject(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sub)
}
