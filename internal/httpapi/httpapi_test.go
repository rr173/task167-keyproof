package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task167-keyproof/internal/service"
	"task167-keyproof/internal/store"
)

func TestCreateKeyRouteUsesRealMux(t *testing.T) {
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(service.New(st)).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewBufferString(`{"name":"root","kind":"root"}`))
	req = req.WithContext(context.Background())
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	srv.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID == "" {
		t.Fatalf("created key response has no id: %s", resp.Body.String())
	}
}
