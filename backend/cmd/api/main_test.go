package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrationDisabledReturnsForbidden(t *testing.T) {
	s := NewServer(nil, false)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"misha","password":"very-long-password","displayName":"Misha"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestInventoryMutationsRequireSession(t *testing.T) {
	s := NewServer(nil, true)
	for _, request := range []struct{ method, path string }{
		{http.MethodPatch, "/api/v1/items/85f35ef6-92a4-4dc8-97be-e844d7a58d4c"},
		{http.MethodPatch, "/api/v1/items/85f35ef6-92a4-4dc8-97be-e844d7a58d4c/media"},
		{http.MethodDelete, "/api/v1/boxes/85f35ef6-92a4-4dc8-97be-e844d7a58d4c"},
		{http.MethodPatch, "/api/v1/locations/85f35ef6-92a4-4dc8-97be-e844d7a58d4c"},
	} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(request.method, request.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: got %d, want %d", request.method, request.path, w.Code, http.StatusUnauthorized)
		}
	}
}
