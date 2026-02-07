package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/gorilla/mux"
)

type Handler struct {
	backend storage.Backend
}

func NewHandler(backend storage.Backend) *Handler {
	return &Handler{
		backend: backend,
	}
}

func (h *Handler) SetupRoutes() *mux.Router {
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Artifact endpoints
	r.HandleFunc("/sites/{site_id}/artifacts/{build_id}", h.StoreArtifact).Methods("PUT")
	r.HandleFunc("/sites/{site_id}/artifacts/{build_id}", h.FetchArtifact).Methods("GET")

	// Log and version endpoints
	r.HandleFunc("/sites/{site_id}/logs", h.WriteLog).Methods("POST")
	r.HandleFunc("/sites/{site_id}/versions", h.ListVersions).Methods("GET")

	return r
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) StoreArtifact(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	siteID := vars["site_id"]
	buildID := vars["build_id"]

	if siteID == "" || buildID == "" {
		http.Error(w, "site_id and build_id are required", http.StatusBadRequest)
		return
	}

	// Read the request body (multipart or direct stream)
	if err := h.backend.StoreArtifact(siteID, buildID, r.Body); err != nil {
		http.Error(w, fmt.Sprintf("Failed to store artifact: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message":  "Artifact stored successfully",
		"site_id":  siteID,
		"build_id": buildID,
	})
}

func (h *Handler) FetchArtifact(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	siteID := vars["site_id"]
	buildID := vars["build_id"]

	if siteID == "" || buildID == "" {
		http.Error(w, "site_id and build_id are required", http.StatusBadRequest)
		return
	}

	reader, err := h.backend.FetchArtifact(siteID, buildID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch artifact: %v", err), http.StatusNotFound)
		return
	}
	defer reader.Close()

	// Set headers for file download
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s-%s.tar.gz", siteID, buildID))

	// Stream the file
	if _, err := io.Copy(w, reader); err != nil {
		// Can't send error at this point, just log it
		fmt.Printf("Error streaming artifact: %v\n", err)
	}
}

type LogRequest struct {
	BuildID  string            `json:"build_id"`
	Action   string            `json:"action"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (h *Handler) WriteLog(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	siteID := vars["site_id"]

	if siteID == "" {
		http.Error(w, "site_id is required", http.StatusBadRequest)
		return
	}

	var req LogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.BuildID == "" || req.Action == "" || req.Status == "" {
		http.Error(w, "build_id, action, and status are required", http.StatusBadRequest)
		return
	}

	entry := &storage.LogEntry{
		Timestamp: time.Now().UTC(),
		BuildID:   req.BuildID,
		SiteID:    siteID,
		Action:    req.Action,
		Status:    req.Status,
		Metadata:  req.Metadata,
	}

	if err := h.backend.WriteLogEntry(siteID, entry); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write log entry: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Log entry written successfully",
		"site_id":   siteID,
		"build_id":  req.BuildID,
		"timestamp": entry.Timestamp.Format(time.RFC3339),
	})
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	siteID := vars["site_id"]

	if siteID == "" {
		http.Error(w, "site_id is required", http.StatusBadRequest)
		return
	}

	versions, err := h.backend.ListVersions(siteID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list versions: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id":  siteID,
		"versions": versions,
		"count":    len(versions),
	})
}
