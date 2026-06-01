// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

// ObjectMeta represents structured fields stored in Extra map[string]any.
// These accessors provide type-safe get/set while maintaining backward
// compatibility with code that reads/writes Extra directly.
type ObjectMeta struct {
	Tags         []string `json:"tags,omitempty"`
	PreviewURL   string   `json:"preview_url,omitempty"`
	LocalPreview string   `json:"local_preview,omitempty"`
	Files        []any    `json:"files,omitempty"`
	Links        []any    `json:"links,omitempty"`
	ContentText  string   `json:"content_text,omitempty"`
	ContentHTML  string   `json:"content_html,omitempty"`
	PageURL      string   `json:"page_url,omitempty"`
	GroupSize    int      `json:"group_size,omitempty"`
	Images       []string `json:"images,omitempty"`
}

// ObjectMetadata represents structured fields stored in Metadata map[string]string.
type ObjectMetadata struct {
	Title        string `json:"title,omitempty"`
	Date         string `json:"date,omitempty"`
	Duration     string `json:"duration,omitempty"`
	ContentGroup string `json:"content_group,omitempty"`
	TaskType     string `json:"task_type,omitempty"`
	PageURL      string `json:"page_url,omitempty"`
}

// --- Extra accessors ---

// GetTags returns tags from Extra, or nil.
func (o *DownloadObject) GetTags() []string {
	if o == nil || o.Extra == nil {
		return nil
	}
	raw, ok := o.Extra["tags"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		tags := make([]string, 0, len(v))
		for _, t := range v {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	}
	return nil
}

// SetTags sets tags in Extra.
func (o *DownloadObject) SetTags(tags []string) {
	if o == nil {
		return
	}
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	if tags == nil {
		delete(o.Extra, "tags")
		return
	}
	o.Extra["tags"] = tags
}

// GetPreviewURL returns preview_url from Extra, or empty string.
func (o *DownloadObject) GetPreviewURL() string {
	if o == nil || o.Extra == nil {
		return ""
	}
	s, _ := o.Extra["preview_url"].(string)
	return s
}

// SetPreviewURL sets preview_url in Extra.
func (o *DownloadObject) SetPreviewURL(url string) {
	if o == nil {
		return
	}
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["preview_url"] = url
}

// GetLocalPreview returns local_preview from Extra, or empty string.
func (o *DownloadObject) GetLocalPreview() string {
	if o == nil || o.Extra == nil {
		return ""
	}
	s, _ := o.Extra["local_preview"].(string)
	return s
}

// SetLocalPreview sets local_preview in Extra.
func (o *DownloadObject) SetLocalPreview(path string) {
	if o == nil {
		return
	}
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["local_preview"] = path
}

// GetGroupSize returns group_size from Extra, or 0.
func (o *DownloadObject) GetGroupSize() int {
	if o == nil || o.Extra == nil {
		return 0
	}
	switch v := o.Extra["group_size"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// SetGroupSize sets group_size in Extra.
func (o *DownloadObject) SetGroupSize(n int) {
	if o == nil {
		return
	}
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["group_size"] = n
}

// GetContentGroup returns content_group from Extra, or empty string.
func (o *DownloadObject) GetContentGroup() string {
	if o == nil || o.Extra == nil {
		return ""
	}
	s, _ := o.Extra["content_group"].(string)
	return s
}

// SetContentGroup sets content_group in Extra.
func (o *DownloadObject) SetContentGroup(group string) {
	if o == nil {
		return
	}
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["content_group"] = group
}

// --- Metadata accessors ---

// GetMetaTitle returns title from Metadata.
func (o *DownloadObject) GetMetaTitle() string {
	if o == nil || o.Metadata == nil {
		return ""
	}
	return o.Metadata["title"]
}

// SetMetaTitle sets title in Metadata.
func (o *DownloadObject) SetMetaTitle(title string) {
	if o == nil {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["title"] = title
}

// GetMetaDate returns date from Metadata.
func (o *DownloadObject) GetMetaDate() string {
	if o == nil || o.Metadata == nil {
		return ""
	}
	return o.Metadata["date"]
}

// SetMetaDate sets date in Metadata.
func (o *DownloadObject) SetMetaDate(date string) {
	if o == nil {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["date"] = date
}

// GetMetaDuration returns duration from Metadata.
func (o *DownloadObject) GetMetaDuration() string {
	if o == nil || o.Metadata == nil {
		return ""
	}
	return o.Metadata["duration"]
}

// SetMetaDuration sets duration in Metadata.
func (o *DownloadObject) SetMetaDuration(dur string) {
	if o == nil {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["duration"] = dur
}

// GetMetaContentGroup returns content_group from Metadata.
func (o *DownloadObject) GetMetaContentGroup() string {
	if o == nil || o.Metadata == nil {
		return ""
	}
	return o.Metadata["content_group"]
}

// SetMetaContentGroup sets content_group in Metadata.
func (o *DownloadObject) SetMetaContentGroup(group string) {
	if o == nil {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["content_group"] = group
}

// GetMetaTaskType returns task_type from Metadata.
func (o *DownloadObject) GetMetaTaskType() string {
	if o == nil || o.Metadata == nil {
		return ""
	}
	return o.Metadata["task_type"]
}

// SetMetaTaskType sets task_type in Metadata.
func (o *DownloadObject) SetMetaTaskType(t string) {
	if o == nil {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["task_type"] = t
}
