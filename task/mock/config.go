// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/cocomhub/download-manager/model"
)

// MockRule defines a rule for generating download objects.
type MockRule struct {
	URLTemplate     string            `yaml:"url_template" json:"url_template"`
	Count           int               `yaml:"count" json:"count"`
	Slugs           []string          `yaml:"slugs,omitempty" json:"slugs,omitempty"`
	FileSize        int64             `yaml:"file_size,omitempty" json:"file_size,omitempty"`
	InitialProgress int               `yaml:"initial_progress,omitempty" json:"initial_progress,omitempty"`
	Status          string            `yaml:"status,omitempty" json:"status,omitempty"`
	Metadata        map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// Validate validates the rule and returns an error if invalid.
func (r *MockRule) Validate() error {
	if r.URLTemplate == "" {
		return fmt.Errorf("url_template is required")
	}
	if r.Count <= 0 && len(r.Slugs) == 0 {
		return fmt.Errorf("count must be > 0 or slugs must be non-empty")
	}
	return nil
}

// generateObjects generates DownloadObjects from the rule.
// taskID is set on each generated object.
func (r *MockRule) generateObjects(taskID string, urlSuffixOffset int) []*model.DownloadObject {
	count := r.Count
	if count <= 0 {
		count = len(r.Slugs)
	}

	var objects []*model.DownloadObject
	for i := 0; i < count; i++ {
		url := r.URLTemplate
		url = strings.ReplaceAll(url, "{n}", strconv.Itoa(urlSuffixOffset+i))
		if len(r.Slugs) > 0 && i < len(r.Slugs) {
			url = strings.ReplaceAll(url, "{slug}", r.Slugs[i])
		}

		obj := &model.DownloadObject{
			TaskID:   taskID,
			URL:      url,
			Metadata: make(map[string]string),
		}

		if r.Status != "" {
			obj.SetStatus(r.Status)
		} else {
			obj.SetStatus(model.StatusPending)
		}
		obj.SetProgress(r.InitialProgress)

		if r.Metadata != nil {
			maps.Copy(obj.Metadata, r.Metadata)
		}

		obj.SetGroupSize(int(r.FileSize))

		objects = append(objects, obj)
	}
	return objects
}

// MockBehavior defines the behavior of the mock downloader.
type MockBehavior struct {
	Mode          string   `yaml:"mode" json:"mode"`
	FailRate      float64  `yaml:"fail_rate,omitempty" json:"fail_rate,omitempty"`
	FailOnURLs    []string `yaml:"fail_on_urls,omitempty" json:"fail_on_urls,omitempty"`
	TimeoutOnURLs []string `yaml:"timeout_on_urls,omitempty" json:"timeout_on_urls,omitempty"`
	DelayPerByte  float64  `yaml:"delay_per_byte,omitempty" json:"delay_per_byte,omitempty"`
}

// parseMockRules extracts []MockRule from a config.Extra map.
func parseMockRules(extra map[string]any) ([]MockRule, error) {
	raw, ok := extra["mock_rules"]
	if !ok {
		return nil, fmt.Errorf("mock_rules not found in extra config")
	}

	rulesRaw, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("mock_rules must be a list")
	}

	var rules []MockRule
	for i, r := range rulesRaw {
		rMap, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mock_rules[%d]: expected map, got %T", i, r)
		}
		rule, err := mapToMockRule(rMap)
		if err != nil {
			return nil, fmt.Errorf("mock_rules[%d]: %w", i, err)
		}
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("mock_rules[%d]: %w", i, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// parseMockBehavior extracts MockBehavior from a config.Extra map.
func parseMockBehavior(extra map[string]any) (MockBehavior, error) {
	mb := MockBehavior{}
	raw, ok := extra["mock_behavior"]
	if !ok {
		return mb, nil // not required
	}

	bMap, ok := raw.(map[string]any)
	if !ok {
		return mb, fmt.Errorf("mock_behavior must be a map")
	}

	if v, ok := bMap["mode"]; ok {
		mb.Mode, _ = v.(string)
	}
	if v, ok := bMap["fail_rate"]; ok {
		mb.FailRate, _ = v.(float64)
	}
	if v, ok := bMap["delay_per_byte"]; ok {
		mb.DelayPerByte, _ = v.(float64)
	}
	if v, ok := bMap["fail_on_urls"]; ok {
		urls, ok := v.([]any)
		if ok {
			for _, u := range urls {
				if s, ok := u.(string); ok {
					mb.FailOnURLs = append(mb.FailOnURLs, s)
				}
			}
		}
	}
	if v, ok := bMap["timeout_on_urls"]; ok {
		urls, ok := v.([]any)
		if ok {
			for _, u := range urls {
				if s, ok := u.(string); ok {
					mb.TimeoutOnURLs = append(mb.TimeoutOnURLs, s)
				}
			}
		}
	}
	return mb, nil
}

func mapToMockRule(m map[string]any) (MockRule, error) {
	r := MockRule{}
	if v, ok := m["url_template"]; ok {
		r.URLTemplate, _ = v.(string)
	}
	if v, ok := m["count"]; ok {
		switch vv := v.(type) {
		case int:
			r.Count = vv
		case float64:
			r.Count = int(vv)
		}
	}
	if v, ok := m["file_size"]; ok {
		switch vv := v.(type) {
		case int64:
			r.FileSize = vv
		case float64:
			r.FileSize = int64(vv)
		case int:
			r.FileSize = int64(vv)
		}
	}
	if v, ok := m["initial_progress"]; ok {
		switch vv := v.(type) {
		case int:
			r.InitialProgress = vv
		case float64:
			r.InitialProgress = int(vv)
		}
	}
	if v, ok := m["status"]; ok {
		r.Status, _ = v.(string)
	}
	if v, ok := m["slugs"]; ok {
		slugsRaw, ok := v.([]any)
		if ok {
			for _, slug := range slugsRaw {
				if s, ok := slug.(string); ok {
					r.Slugs = append(r.Slugs, s)
				}
			}
		}
	}
	if v, ok := m["metadata"]; ok {
		metaRaw, ok := v.(map[string]any)
		if ok {
			r.Metadata = make(map[string]string)
			for k, vv := range metaRaw {
				r.Metadata[k], _ = vv.(string)
			}
		}
	}
	return r, nil
}
