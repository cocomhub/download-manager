// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"math/rand"
	"slices"
	"sort"
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// ApplyQueryToObjects applies the shared Storage query semantics to an object slice.
func ApplyQueryToObjects(objects []*model.DownloadObject, query *core.StorageQuery) []*model.DownloadObject {
	if len(objects) == 0 {
		return []*model.DownloadObject{}
	}

	filtered := make([]*model.DownloadObject, 0, len(objects))
	for _, obj := range objects {
		if matchesQuery(obj, query) {
			filtered = append(filtered, obj)
		}
	}

	applySort(filtered, query)
	return applyPagination(filtered, query)
}

// CountObjects returns the count of objects that match the query filters.
func CountObjects(objects []*model.DownloadObject, query *core.StorageQuery) int64 {
	var count int64
	for _, obj := range objects {
		if matchesQuery(obj, query) {
			count++
		}
	}
	return count
}

func matchesFilterFields(obj *model.DownloadObject, filter core.StorageFilter) bool {
	if len(filter.TaskIDs) > 0 && !containsString(filter.TaskIDs, obj.TaskID) {
		return false
	}
	if len(filter.URLs) > 0 && !containsString(filter.URLs, obj.URL) {
		return false
	}
	if len(filter.Statuses) > 0 && !containsString(filter.Statuses, obj.GetStatus()) {
		return false
	}
	if filter.MissingID != nil {
		id := obj.GetID()
		if *filter.MissingID && id != 0 {
			return false
		}
		if !*filter.MissingID && id == 0 {
			return false
		}
	}

	// Tags 过滤
	if len(filter.Tags) > 0 {
		obj.RLock()
		matched := matchTags(obj.Extra, filter.Tags, filter.TagMode)
		obj.RUnlock()
		if !matched {
			return false
		}
	}

	// ExcludeIDs 过滤
	if len(filter.ExcludeIDs) > 0 {
		obj.RLock()
		id := obj.ID
		obj.RUnlock()
		if slices.Contains(filter.ExcludeIDs, id) {
			return false
		}
	}

	return true
}

func matchesQuery(obj *model.DownloadObject, query *core.StorageQuery) bool {
	if obj == nil {
		return false
	}
	if query == nil {
		return true
	}
	if !matchesFilterFields(obj, query.Filter) {
		return false
	}

	// Lock the object to safely read Metadata and Extra maps,
	// which may be concurrently written by applySharedState (base_task.go).
	obj.RLock()
	defer obj.RUnlock()

	if len(query.Filter.Metadata) > 0 {
		for key, want := range query.Filter.Metadata {
			if obj.Metadata == nil || obj.Metadata[key] != want {
				return false
			}
		}
	}

	search := strings.ToLower(strings.TrimSpace(query.Filter.Search))
	if search == "" {
		return true
	}
	if strings.Contains(strings.ToLower(obj.URL), search) {
		return true
	}
	if obj.Metadata != nil && strings.Contains(strings.ToLower(obj.Metadata[model.MetadataKeyTitle]), search) {
		return true
	}
	return extraTagsContain(obj.Extra, search)
}

func extraTagsContain(extra map[string]any, search string) bool {
	if len(extra) == 0 {
		return false
	}
	raw, ok := extra["tags"]
	if !ok {
		return false
	}
	switch tags := raw.(type) {
	case []string:
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), search) {
				return true
			}
		}
	case []any:
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok && strings.Contains(strings.ToLower(tagStr), search) {
				return true
			}
		}
	case string:
		return strings.Contains(strings.ToLower(tags), search)
	}
	return false
}

// matchTags checks if the object's extra.tags match the filter tags based on the mode.
// mode "all" requires all filter tags to be present (AND), mode "any" requires at least one (OR).
func matchTags(extra map[string]any, tags []string, mode string) bool {
	if len(extra) == 0 || len(tags) == 0 {
		return true
	}
	raw, ok := extra["tags"]
	if !ok {
		return false
	}
	var objTags []string
	switch t := raw.(type) {
	case []string:
		objTags = t
	case []any:
		for _, tag := range t {
			if s, ok := tag.(string); ok {
				objTags = append(objTags, s)
			}
		}
	}

	objTagSet := make(map[string]bool, len(objTags))
	for _, t := range objTags {
		objTagSet[strings.ToLower(t)] = true
	}

	if mode == "all" {
		for _, tag := range tags {
			if !objTagSet[strings.ToLower(tag)] {
				return false
			}
		}
		return true
	}
	// default: "any"
	for _, tag := range tags {
		if objTagSet[strings.ToLower(tag)] {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func compareStrings(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func applySort(objects []*model.DownloadObject, query *core.StorageQuery) {
	if len(objects) < 2 || query == nil || len(query.Sort) == 0 {
		return
	}

	// Check for random sort — shuffle in place and return immediately
	for _, rule := range query.Sort {
		if rule.Field == "random" {
			rand.Shuffle(len(objects), func(i, j int) {
				objects[i], objects[j] = objects[j], objects[i]
			})
			return
		}
	}

	sort.SliceStable(objects, func(i, j int) bool {
		left := objects[i]
		right := objects[j]
		for _, rule := range query.Sort {
			cmp := compareByField(left, right, rule.Field)
			if cmp == 0 {
				continue
			}
			if rule.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

func compareByField(left, right *model.DownloadObject, field string) int {
	switch field {
	case "date":
		return compareStrings(metadataValue(left, "date"), metadataValue(right, "date"))
	case "name":
		leftName := metadataValue(left, "title")
		if leftName == "" {
			leftName = objectURL(left)
		}
		rightName := metadataValue(right, "title")
		if rightName == "" {
			rightName = objectURL(right)
		}
		return compareStrings(strings.ToLower(leftName), strings.ToLower(rightName))
	case "duration":
		return compareStrings(metadataValue(left, "duration"), metadataValue(right, "duration"))
	case "status":
		return compareStrings(objectStatus(left), objectStatus(right))
	case "url":
		return compareStrings(objectURL(left), objectURL(right))
	default:
		// random 和 tag_match_desc 排序由 applySort 特殊处理，
		// 不在 compareByField 中实现。
		return 0
	}
}

func applyPagination(objects []*model.DownloadObject, query *core.StorageQuery) []*model.DownloadObject {
	if len(objects) == 0 {
		return []*model.DownloadObject{}
	}
	if query == nil {
		return objects
	}

	offset := max(query.Offset, 0)
	if offset >= int64(len(objects)) {
		return []*model.DownloadObject{}
	}
	if query.Limit <= 0 {
		return objects[offset:]
	}
	end := min(offset+query.Limit, int64(len(objects)))
	return objects[offset:end]
}

func metadataValue(obj *model.DownloadObject, key string) string {
	if obj == nil {
		return ""
	}
	// Lock to safely read Metadata, which may be concurrently
	// written by applySharedState (base_task.go).
	obj.RLock()
	defer obj.RUnlock()
	if obj.Metadata == nil {
		return ""
	}
	return obj.Metadata[key]
}

func objectURL(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	return obj.URL
}

func objectStatus(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	return obj.GetStatus()
}
