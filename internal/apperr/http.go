package apperr

import (
	"encoding/json"
	"net/http"
)

// WriteHTTPError writes a JSON error response {"error": msg} with the given HTTP
// status. The message is properly JSON-escaped, unlike fmt.Sprintf-ing into a
// JSON literal (which is vulnerable to injection/breakage when msg contains
// quotes, backslashes, or control characters — e.g. an underlying error string).
func WriteHTTPError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
