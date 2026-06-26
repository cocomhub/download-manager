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
	mb.FailOnURLs = extractStringList(bMap, "fail_on_urls")
	mb.TimeoutOnURLs = extractStringList(bMap, "timeout_on_urls")
	return mb, nil
}

// extractStringList extracts a []string value for a key from a map.
func extractStringList(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	return stringSliceFromAny(v)
}

func mapToMockRule(m map[string]any) (MockRule, error) {
	r := MockRule{}
	r.URLTemplate, _ = m["url_template"].(string)
	r.Count = intFromAny(m["count"])
	r.FileSize = int64FromAny(m["file_size"])
	r.InitialProgress = intFromAny(m["initial_progress"])
	r.Status, _ = m["status"].(string)
	r.Slugs = stringSliceFromAny(m["slugs"])
	r.Metadata = stringMapFromAny(m["metadata"])
	return r, nil
}

func intFromAny(v any) int {
	switch vv := v.(type) {
	case int:
		return vv
	case float64:
		return int(vv)
	}
	return 0
}

func int64FromAny(v any) int64 {
	switch vv := v.(type) {
	case int64:
		return vv
	case float64:
		return int64(vv)
	case int:
		return int64(vv)
	}
	return 0
}

func stringSliceFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func stringMapFromAny(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string)
	for k, vv := range raw {
		result[k], _ = vv.(string)
	}
	return result
}
