package apperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteHTTPErrorEscapes(t *testing.T) {
	t.Parallel()
	// A message containing characters that would break a naive JSON literal.
	evil := `bad" }{ "extra": "injected`
	rr := httptest.NewRecorder()
	WriteHTTPError(rr, http.StatusBadRequest, evil)

	if got := rr.Code; got != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not valid JSON: %v (body: %q)", err, rr.Body.String())
	}
	if body["error"] != evil {
		t.Errorf("error field = %q, want %q", body["error"], evil)
	}
	// The injection attempt must NOT have produced a leaked "extra" key — the
	// whole payload must be a single escaped "error" string value.
	if _, leaked := body["extra"]; leaked {
		t.Errorf("response leaked injected key: %q", rr.Body.String())
	}
	if len(body) != 1 {
		t.Errorf("response has %d keys, want 1: %q", len(body), rr.Body.String())
	}
}

func TestWriteHTTPErrorStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError} {
		rr := httptest.NewRecorder()
		WriteHTTPError(rr, status, "boom")
		if rr.Code != status {
			t.Errorf("status = %d, want %d", rr.Code, status)
		}
	}
}
