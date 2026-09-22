package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMediaEndpointRequiresSession(t *testing.T) {
	s := NewServer(nil, true)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/media/85f35ef6-92a4-4dc8-97be-e844d7a58d4c", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Fatalf("media must not redirect to object storage, got Location=%q", got)
	}
}
