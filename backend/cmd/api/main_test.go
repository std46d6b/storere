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
