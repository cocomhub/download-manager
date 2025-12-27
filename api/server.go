package api

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"download-manager/config"
	"download-manager/manager"
	"download-manager/web"

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
	r.HandleFunc("/api/tasks/{id}/retry", s.retryTask).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/reorder", s.reorderTask).Methods("POST")
	r.HandleFunc("/api/config", s.getConfig).Methods("GET")
	r.HandleFunc("/api/config", s.updateConfig).Methods("POST")
	r.HandleFunc("/api/downloads", s.getActiveDownloads).Methods("GET")
	r.HandleFunc("/api/events", s.handleEvents).Methods("GET")

	// File Preview Route
	// Assuming files are in build/test/downloads based on recent config changes
	// In a real app, this path should be configurable or dynamic per task
	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir("./downloads"))))

	// Static UI
	subFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal("Failed to embed static files:", err)
	}
	r.PathPrefix("/").Handler(http.FileServer(http.FS(subFS)))

	return r
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.mgr.GetTaskSummaries()
	json.NewEncoder(w).Encode(tasks)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	page := 1
	limit := 50 // Default

	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if lStr == "all" {
			limit = -1
		} else if l, err := strconv.Atoi(lStr); err == nil {
			limit = l
		}
	}

	search := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort")

	details, err := s.mgr.GetTaskDetails(id, page, limit, search, sortBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(details)
}

type RetryRequest struct {
	URL string `json:"url"` // Optional, if empty retry all failed
}

func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req RetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body for "retry all failed"
	}

	if req.URL != "" {
		if err := s.mgr.RetryObject(id, req.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := s.mgr.RetryAllFailed(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

type ReorderRequest struct {
	URL      string `json:"url"`
	NewIndex int    `json:"new_index"`
}

func (s *Server) reorderTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	if err := s.mgr.ReorderObject(id, req.URL, req.NewIndex); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.mgr.UpdateConfig(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) getActiveDownloads(w http.ResponseWriter, r *http.Request) {
	actives := s.mgr.GetActiveDownloads()
	json.NewEncoder(w).Encode(actives)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Flush immediately to establish connection
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	// Subscribe to manager events
	eventChan := s.mgr.Subscribe()
	defer s.mgr.Unsubscribe(eventChan)

	// Listen for client disconnect
	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case event := <-eventChan:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			// SSE format: "data: ...\n\n"
			// Using "event" field for type if needed, but we put type in payload
			// Let's just send data
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
