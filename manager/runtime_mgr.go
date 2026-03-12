package manager

import (
	"fmt"
	"log/slog"

	"download-manager/config"
)

func (m *Manager) worker() {
	defer m.workerWg.Done()
	for {
		select {
		case req, ok := <-m.downloadQueue:
			if !ok {
				return
			}
			if req != nil {
				m.download(req.task, req.obj)
			}
		case <-m.stopChan:
			return
		case <-m.workerStop:
			return
		}
	}
}

func (m *Manager) adjustGlobalWorkers(newLimit int) {
	if newLimit <= 0 {
		newLimit = 5
	}
	if newLimit > m.workerCount {
		add := newLimit - m.workerCount
		slog.Info("Increasing global workers", "from", m.workerCount, "to", newLimit)
		for range add {
			m.workerWg.Add(1)
			go m.worker()
		}
		m.workerCount = newLimit
	} else if newLimit < m.workerCount {
		remove := m.workerCount - newLimit
		slog.Info("Decreasing global workers", "from", m.workerCount, "to", newLimit)
		for range remove {
			select {
			case m.workerStop <- struct{}{}:
			default:
				m.workerStop <- struct{}{}
			}
		}
		m.workerCount = newLimit
	}
}

func (m *Manager) applyTaskRuntime(newCfg *config.Config) {
	for _, tCfg := range newCfg.Tasks {
		if t, ok := m.getTask(tCfg.ID); ok {
			if setter, ok := t.(interface{ SetConcurrency(int) error }); ok {
				var cfgVal int
				if tCfg.Extra != nil {
					if v, ok := tCfg.Extra["max_concurrent"].(int); ok {
						cfgVal = v
					} else if v, ok := tCfg.Extra["max_concurrent"].(float64); ok {
						cfgVal = int(v)
					}
				}
				if cfgVal > 0 {
					if getter, ok := t.(interface{ GetConcurrency() int }); !ok || getter.GetConcurrency() != cfgVal {
						if err := setter.SetConcurrency(cfgVal); err != nil {
							slog.Warn("SetConcurrency failed", "task_id", tCfg.ID, "error", err)
						} else {
							slog.Info("Task concurrency updated", "task_id", tCfg.ID, "value", cfgVal)
						}
					}
				}
			}
			if setter, ok := t.(interface{ SetRefreshInterval(int) error }); ok {
				var cfgVal int
				if tCfg.Extra != nil {
					if v, ok := tCfg.Extra["refresh_interval"].(int); ok {
						cfgVal = v
					} else if v, ok := tCfg.Extra["refresh_interval"].(float64); ok {
						cfgVal = int(v)
					}
				}
				if cfgVal > 0 {
					if getter, ok := t.(interface{ GetRefreshInterval() int }); !ok || getter.GetRefreshInterval() != cfgVal {
						if err := setter.SetRefreshInterval(cfgVal); err != nil {
							slog.Warn("SetRefreshInterval failed", "task_id", tCfg.ID, "error", err)
						} else {
							slog.Info("Task refresh interval updated", "task_id", tCfg.ID, "value", cfgVal)
						}
					}
				}
			}
		}
	}
}

func (m *Manager) SetTaskConfig(taskID string, concurrency *int, refreshInterval *int, audit *AuditInfo) (map[string]bool, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	result := map[string]bool{"concurrency": false, "refresh_interval": false}
	if concurrency != nil {
		if setter, ok := t.(interface{ SetConcurrency(int) error }); ok {
			if err := setter.SetConcurrency(*concurrency); err != nil {
				return result, err
			}
			result["concurrency"] = true
		}
	}
	if refreshInterval != nil {
		if setter, ok := t.(interface{ SetRefreshInterval(int) error }); ok {
			if err := setter.SetRefreshInterval(*refreshInterval); err != nil {
				return result, err
			}
			result["refresh_interval"] = true
		}
	}
	if result["concurrency"] || result["refresh_interval"] {
		// Persist to config file
		m.mu.Lock()
		cfgCopy := *m.cfg
		for i := range cfgCopy.Tasks {
			if cfgCopy.Tasks[i].ID == taskID {
				if cfgCopy.Tasks[i].Extra == nil {
					cfgCopy.Tasks[i].Extra = make(map[string]any)
				}
				if concurrency != nil {
					cfgCopy.Tasks[i].Extra["max_concurrent"] = *concurrency
				}
				if refreshInterval != nil {
					cfgCopy.Tasks[i].Extra["refresh_interval"] = *refreshInterval
				}
				break
			}
		}
		m.mu.Unlock()
		return result, m.UpdateConfig(&cfgCopy, audit)
	}
	return result, nil
}
