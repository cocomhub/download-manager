package api

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"download-manager/config"
	"download-manager/logutil"
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
	r.HandleFunc("/api/tasks", s.createTaskPersistent).Methods("POST")
	r.HandleFunc("/api/tasks/{id}", s.getTask).Methods("GET")
	r.HandleFunc("/api/tasks/{id}", s.updateTaskPersistent).Methods("PUT")
	r.HandleFunc("/api/tasks/{id}/retry", s.retryTask).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/reorder", s.reorderTask).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/config", s.updateTaskConfig).Methods("POST")
	r.HandleFunc("/api/config/server", s.getServerConfig).Methods("GET")
	r.HandleFunc("/api/config/server", s.updateServerConfig).Methods("POST")
	r.HandleFunc("/api/config/log", s.getLogConfig).Methods("GET")
	r.HandleFunc("/api/config/log", s.updateLogConfig).Methods("POST")
	r.HandleFunc("/api/config/history", s.listConfigHistory).Methods("GET")
	r.HandleFunc("/api/config/rollback", s.rollbackConfig).Methods("POST")
	r.HandleFunc("/api/config/diff", s.diffConfig).Methods("GET")
	r.HandleFunc("/api/config/tag", s.addConfigTag).Methods("POST")
	r.HandleFunc("/api/config/note", s.addConfigNote).Methods("POST")
	r.HandleFunc("/api/downloads", s.getActiveDownloads).Methods("GET")
	r.HandleFunc("/api/events", s.handleEvents).Methods("GET")

	// File Preview Route
	// Assuming files are in build/test/downloads based on recent config changes
	// In a real app, this path should be configurable or dynamic per task
	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir(s.mgr.GetDownloadRootDir()))))

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

type TaskConfigRequest struct {
	Concurrency     *int `json:"concurrency"`
	RefreshInterval *int `json:"refresh_interval"`
}

func (s *Server) updateTaskConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req TaskConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	applied, err := s.mgr.SetTaskConfig(id, req.Concurrency, req.RefreshInterval)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"applied": applied,
	})
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

func (s *Server) listConfigHistory(w http.ResponseWriter, r *http.Request) {
	h, _ := s.mgr.ListConfigBackups()
	json.NewEncoder(w).Encode(h)
}

type RollbackRequest struct {
	Filename string `json:"filename"`
}

func (s *Server) rollbackConfig(w http.ResponseWriter, r *http.Request) {
	var req RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.mgr.RollbackConfig(req.Filename); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
func (s *Server) diffConfig(w http.ResponseWriter, r *http.Request) {
	left := r.URL.Query().Get("left")
	right := r.URL.Query().Get("right")
	ignoreWS := r.URL.Query().Get("ignore_ws") == "1"
	ignoreComments := r.URL.Query().Get("ignore_comments") == "1"
	res, err := s.mgr.DiffConfigFilesOpts(left, right, ignoreWS, ignoreComments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(res)
}

func (s *Server) addConfigTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Tag      string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.Tag == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.mgr.AddConfigTag(req.Filename, req.Tag); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) addConfigNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Message  string `json:"message"`
		Author   string `json:"author"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.Message == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.mgr.AddConfigNote(req.Filename, req.Message, req.Author); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Persistent Task Management (create/update via config)
func (s *Server) createTaskPersistent(w http.ResponseWriter, r *http.Request) {
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if t.ID == "" || t.Type == "" {
		http.Error(w, "id and type are required", http.StatusBadRequest)
		return
	}
	cur := s.mgr.GetConfig()
	// prevent duplicate
	for _, existing := range cur.Tasks {
		if existing.ID == t.ID {
			http.Error(w, "task id already exists", http.StatusConflict)
			return
		}
	}
	cur.Tasks = append(cur.Tasks, t)
	if err := s.mgr.UpdateConfig(cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) updateTaskPersistent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cur := s.mgr.GetConfig()
	found := false
	for i := range cur.Tasks {
		if cur.Tasks[i].ID == id {
			cur.Tasks[i] = t
			cur.Tasks[i].ID = id
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err := s.mgr.UpdateConfig(cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
func (s *Server) getServerConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	resp := map[string]interface{}{
		"task_scan": map[string]interface{}{
			"disable":  cfg.TaskScan.Disable,
			"interval": cfg.TaskScan.Interval,
		},
		"downloader": map[string]interface{}{
			"proxies":           cfg.Downloader.Proxies,
			"global_concurrent": cfg.Downloader.GlobalConcurrent,
			"force_proxy":       cfg.Downloader.ForceProxy,
			"max_retries":       cfg.Downloader.MaxRetries,
			"type":              cfg.Downloader.Type,
		},
		"ui_defaults": map[string]interface{}{
			"default_save_dir":    cfg.Server.UIDefaults.DefaultSaveDir,
			"window_width":        cfg.Server.UIDefaults.WindowWidth,
			"window_height":       cfg.Server.UIDefaults.WindowHeight,
			"diff_side_by_side":   cfg.Server.UIDefaults.DiffSideBySide,
			"diff_ignore_ws":      cfg.Server.UIDefaults.DiffIgnoreWS,
			"diff_ignore_comment": cfg.Server.UIDefaults.DiffIgnoreComment,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskScan   config.TaskScan   `json:"task_scan"`
		Downloader config.Downloader `json:"downloader"`
		UIDefaults config.UIDefaults `json:"ui_defaults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cur := s.mgr.GetConfig()
	cur.TaskScan.Disable = req.TaskScan.Disable
	cur.TaskScan.Interval = req.TaskScan.Interval
	cur.Downloader.Proxies = req.Downloader.Proxies
	if req.Downloader.GlobalConcurrent > 0 {
		cur.Downloader.GlobalConcurrent = req.Downloader.GlobalConcurrent
	}
	cur.Downloader.ForceProxy = req.Downloader.ForceProxy
	if req.Downloader.MaxRetries > 0 {
		cur.Downloader.MaxRetries = req.Downloader.MaxRetries
	}
	if req.Downloader.Type != "" {
		cur.Downloader.Type = req.Downloader.Type
	}
	cur.Server.UIDefaults = req.UIDefaults
	if err := s.mgr.UpdateConfig(cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getLogConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	json.NewEncoder(w).Encode(cfg.Log)
}

func (s *Server) updateLogConfig(w http.ResponseWriter, r *http.Request) {
	var newLog logutil.LogConfig
	if err := json.NewDecoder(r.Body).Decode(&newLog); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.mgr.UpdateLogConfig(newLog); err != nil {
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
