package http_test

import (
	"net/http"
	"testing"

	"github.com/getappbox/middleware/internal/middleware"
)

func TestProductionConstructorBootsWithoutAPISecrets(t *testing.T) {
	handler := middleware.New(middleware.Config{}, nil)
	health := send(handler, http.MethodGet, "/", "", "", nil)
	requireStatus(t, health, http.StatusOK)
	if health.Body.String() != "AppBox Install Service Helper" {
		t.Fatal(health.Body.String())
	}
	requireStatus(t, send(handler, http.MethodGet, "/api/v1/config", "", "", nil), http.StatusUnauthorized)
}
