package api

import (
	"encoding/json"
	"net/http"

	"download-manager/manager"

	"github.com/gorilla/mux"
)

type Server struct {
	mgr *manager.Manager
}

func NewServer(mgr *manager.Manager) *Server {
	return &Server{mgr: mgr}
}

func (s *Server) Router() *mux.Router {
	r := mux.NewRouter()

	// API Routes
	r.HandleFunc("/api/tasks", s.listTasks).Methods("GET")
	r.HandleFunc("/api/tasks/{id}", s.getTask).Methods("GET")

	// File Preview Route
	// Assuming files are in build/test/downloads based on recent config changes
	// In a real app, this path should be configurable or dynamic per task
	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir("./build/test/downloads"))))

	// Static UI
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/static")))

	return r
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	// Since Manager doesn't expose tasks directly, we might need to extend Manager
	// or just inspect the config + internal state if we expose it.
	// For now, let's expose a method in Manager to get Task Summaries.
	tasks := s.mgr.GetTaskSummaries()
	json.NewEncoder(w).Encode(tasks)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	details, err := s.mgr.GetTaskDetails(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(details)
}
