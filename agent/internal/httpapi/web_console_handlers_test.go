package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyJSONRPCIsHiddenByDefault(t *testing.T) {
	api := &API{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/jsonrpc/0", nil)
	api.jsonRPCProxy(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
