package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
	"github.com/gorilla/mux"
)

type Server struct {
	port     int
	executor *codex.Executor
	status   *types.WorkerStatus
	mu       sync.RWMutex
}

func NewServer(port int, executor *codex.Executor) *Server {
	return &Server{
		port:     port,
		executor: executor,
		status: &types.WorkerStatus{
			State:       "idle",
			CurrentStep: "waiting",
			Progress:    0,
		},
	}
}

func (s *Server) SetupRoutes() *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", s.HealthCheck).Methods("GET")
	r.HandleFunc("/status", s.GetStatus).Methods("GET")
	r.HandleFunc("/kill", s.KillCodex).Methods("POST")

	return r
}

func (s *Server) Start() error {
	router := s.SetupRoutes()
	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("Worker HTTP server starting on %s\n", addr)
	return http.ListenAndServe(addr, router)
}

func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := *s.status
	status.CodexRunning = s.executor.IsRunning()
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) KillCodex(w http.ResponseWriter, r *http.Request) {
	if err := s.executor.Kill(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to kill codex: %v", err), http.StatusInternalServerError)
		return
	}

	s.UpdateStatus("failed", "Execution cancelled by manager", 0)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Codex execution terminated",
	})
}

func (s *Server) UpdateStatus(state, step string, progress int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.State = state
	s.status.CurrentStep = step
	s.status.Progress = progress
}

func (s *Server) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.State = "failed"
	s.status.Error = err.Error()
}
