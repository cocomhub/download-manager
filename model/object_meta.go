// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

// 媒体类型（cover/thumb/preview）的固定关系词汇表，与 core.SmallObjectInfo.Rel 一致。
// 每个媒体类型在 Extra 中有两个固定字段：{rel}_url（原始源 URL）与 {rel}_path（本地保存路径）。
const (
	MediaRelCover   = "cover"
	MediaRelThumb   = "thumb"
	MediaRelPreview = "preview"

	MediaKeyCoverURL    = "cover_url"
	MediaKeyCoverPath   = "cover_path"
	MediaKeyThumbURL    = "thumb_url"
	MediaKeyThumbPath   = "thumb_path"
	MediaKeyPreviewURL  = "preview_url"
	MediaKeyPreviewPath = "preview_path"
)

// validMediaRel 校验 rel 是否为受支持的媒体类型。
func validMediaRel(rel string) bool {
	switch rel {
	case MediaRelCover, MediaRelThumb, MediaRelPreview:
		return true
	}
	return false
}

// ObjectMeta represents structured fields stored in Extra map[string]any.
// These accessors provide type-safe get/set while maintaining backward
// compatibility with code that reads/writes Extra directly.
type ObjectMeta struct {
	Tags         []string `json:"tags,omitempty"`
	CoverURL     string   `json:"cover_url,omitempty"`
	CoverPath    string   `json:"cover_path,omitempty"`
	ThumbURL     string   `json:"thumb_url,omitempty"`
	ThumbPath    string   `json:"thumb_path,omitempty"`
	PreviewURL   string   `json:"preview_url,omitempty"`
	PreviewPath  string   `json:"preview_path,omitempty"`
	LocalCover   string   `json:"local_cover,omitempty"`
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
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
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
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["preview_url"] = url
}

// GetLocalPreview returns local_preview from Extra, or empty string.
func (o *DownloadObject) GetLocalPreview() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["local_preview"] = path
}

// GetGroupSize returns group_size from Extra, or 0.
func (o *DownloadObject) GetGroupSize() int {
	if o == nil {
		return 0
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["group_size"] = n
}

// GetContentGroup returns content_group from Extra, or empty string.
func (o *DownloadObject) GetContentGroup() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra["content_group"] = group
}

// --- 媒体（cover/thumb/preview）固定字段访问器 ---
// 每个媒体类型在 Extra 中保存两个固定 key：{rel}_url（原始源 URL）与 {rel}_path（本地保存路径）。
// 空值会删除对应 key，避免残留空串。

func (o *DownloadObject) SetMedia(rel, url, path string) {
	if o == nil || !validMediaRel(rel) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	setOrDeleteExtra(o.Extra, rel+"_url", url)
	setOrDeleteExtra(o.Extra, rel+"_path", path)
}

// GetMedia 返回某媒体类型的原始 URL 与本地路径。
func (o *DownloadObject) GetMedia(rel string) (url, path string) {
	if o == nil || !validMediaRel(rel) {
		return "", ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
		return "", ""
	}
	url, _ = o.Extra[rel+"_url"].(string)
	path, _ = o.Extra[rel+"_path"].(string)
	return url, path
}

// GetMediaURL 返回某媒体类型的原始源 URL。
func (o *DownloadObject) GetMediaURL(rel string) string {
	url, _ := o.GetMedia(rel)
	return url
}

// GetMediaPath 返回某媒体类型的本地保存路径。
func (o *DownloadObject) GetMediaPath(rel string) string {
	_, path := o.GetMedia(rel)
	return path
}

func (o *DownloadObject) GetCoverURL() string { return o.GetMediaURL(MediaRelCover) }
func (o *DownloadObject) SetCoverURL(url string) {
	_, p := o.GetMedia(MediaRelCover)
	o.SetMedia(MediaRelCover, url, p)
}
func (o *DownloadObject) GetCoverPath() string { return o.GetMediaPath(MediaRelCover) }
func (o *DownloadObject) SetCoverPath(path string) {
	u, _ := o.GetMedia(MediaRelCover)
	o.SetMedia(MediaRelCover, u, path)
}

func (o *DownloadObject) GetThumbURL() string { return o.GetMediaURL(MediaRelThumb) }
func (o *DownloadObject) SetThumbURL(url string) {
	_, p := o.GetMedia(MediaRelThumb)
	o.SetMedia(MediaRelThumb, url, p)
}
func (o *DownloadObject) GetThumbPath() string { return o.GetMediaPath(MediaRelThumb) }
func (o *DownloadObject) SetThumbPath(path string) {
	u, _ := o.GetMedia(MediaRelThumb)
	o.SetMedia(MediaRelThumb, u, path)
}

func (o *DownloadObject) GetPreviewPath() string { return o.GetMediaPath(MediaRelPreview) }
func (o *DownloadObject) SetPreviewPath(path string) {
	u, _ := o.GetMedia(MediaRelPreview)
	o.SetMedia(MediaRelPreview, u, path)
}

// GetLocalCover 返回旧的 local_cover 兼容字段（封面/缩略图本地路径）。
func (o *DownloadObject) GetLocalCover() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Extra == nil {
		return ""
	}
	s, _ := o.Extra["local_cover"].(string)
	return s
}

// SetLocalCover 设置旧的 local_cover 兼容字段。
func (o *DownloadObject) SetLocalCover(path string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	setOrDeleteExtra(o.Extra, "local_cover", path)
}

// setOrDeleteExtra 写入或删除（值为空时）Extra 中的字符串字段。
func setOrDeleteExtra(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
		return
	}
	m[key] = value
}

// --- Metadata accessors ---

// GetMetaTitle returns title from Metadata.
func (o *DownloadObject) GetMetaTitle() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Metadata == nil {
		return ""
	}
	return o.Metadata[MetadataKeyTitle]
}

// SetMetaTitle sets title in Metadata.
func (o *DownloadObject) SetMetaTitle(title string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata[MetadataKeyTitle] = title
}

// GetMetaDate returns date from Metadata.
func (o *DownloadObject) GetMetaDate() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Metadata == nil {
		return ""
	}
	return o.Metadata["date"]
}

// SetMetaDate sets date in Metadata.
func (o *DownloadObject) SetMetaDate(date string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["date"] = date
}

// GetMetaDuration returns duration from Metadata.
func (o *DownloadObject) GetMetaDuration() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Metadata == nil {
		return ""
	}
	return o.Metadata["duration"]
}

// SetMetaDuration sets duration in Metadata.
func (o *DownloadObject) SetMetaDuration(dur string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["duration"] = dur
}

// GetMetaContentGroup returns content_group from Metadata.
func (o *DownloadObject) GetMetaContentGroup() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Metadata == nil {
		return ""
	}
	return o.Metadata[MetadataKeyContentGroup]
}

// SetMetaContentGroup sets content_group in Metadata.
func (o *DownloadObject) SetMetaContentGroup(group string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata[MetadataKeyContentGroup] = group
}

// GetMetaTaskType returns task_type from Metadata.
func (o *DownloadObject) GetMetaTaskType() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.Metadata == nil {
		return ""
	}
	return o.Metadata["task_type"]
}

// SetMetaTaskType sets task_type in Metadata.
func (o *DownloadObject) SetMetaTaskType(t string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["task_type"] = t
}

// EnsureTaskType sets task_type in Metadata if it is not already set.
func (o *DownloadObject) EnsureTaskType(taskType string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Metadata != nil && o.Metadata["task_type"] != "" {
		return
	}
	if o.Metadata == nil {
		o.Metadata = make(map[string]string)
	}
	o.Metadata["task_type"] = taskType
}
