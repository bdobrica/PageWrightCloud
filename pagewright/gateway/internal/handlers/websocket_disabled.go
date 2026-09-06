package handlers

import "net/http"

// WebSocketDisabled is intentionally public and does not read query/header
// credentials. Re-enabling upgrades requires the gate in docs/JOB_HISTORY_API.md.
func WebSocketDisabled(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respondError(w, http.StatusNotImplemented, "WebSockets are disabled; use owner-checked job history polling")
}
