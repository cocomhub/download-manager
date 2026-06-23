// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/model"
)

type StorageFilter struct {
	TaskIDs  []string
	URLs     []string
	Statuses []string
	Metadata map[string]string
	Search   string
}

type StorageSort struct {
	Field string
	Desc  bool
}

type StorageQuery struct {
	Filter StorageFilter
	Sort   []StorageSort
	Offset int64
	Limit  int64
}

// Storage 瀹氫箟涓嬭浇鐘舵€佸瓨鍌ㄧ殑琛屼负
type Storage interface {
	// Get 鑾峰彇鍗曚釜瀵硅薄鐘舵€?	Get(id string) (*model.DownloadObject, error)
	// Update 鏇存柊鍗曚釜瀵硅薄鐨勭姸鎬?	Update(obj *model.DownloadObject) error
	// Delete 鍒犻櫎瀵硅薄鐘舵€?	Delete(id string) error
	// Search 鎸夋煡璇㈡潯浠舵悳绱㈠璞°€?	// nil 鏌ヨ琛ㄧず涓嶈繃婊ゃ€佷笉鎺掑簭銆佷笉鍒嗛〉锛岃繑鍥炲綋鍓嶅瓨鍌ㄤ腑鐨勫叏閮ㄥ璞°€?	Search(query *StorageQuery) ([]*model.DownloadObject, error)
	// Count 杩斿洖鍖归厤鏌ヨ杩囨护鏉′欢鐨勫璞℃€绘暟锛屽拷鐣ユ帓搴忎笌鍒嗛〉鍙傛暟銆?	Count(query *StorageQuery) (int64, error)
	// Exists 鎵归噺妫€鏌ョ粰瀹氬璞?ID 鏄惁瀛樺湪銆?	Exists(ids []string) (map[string]bool, error)
}

// Task 瀹氫箟涓嬭浇浠诲姟鐨勮涓?type Task interface {
	// ID 杩斿洖浠诲姟鍞竴鏍囪瘑
	ID() string
	// Type 杩斿洖浠诲姟绫诲瀷
	Type() string
	// Logger 鏃ュ織
	Logger() *slog.Logger
	// Storage 杩斿洖浠诲姟鐨勫瓨鍌ㄥ悗绔?	Storage() Storage
	// SetDownloader 璁剧疆涓嬭浇鍣?	SetDownloader(d Downloader)
	// GetDownloadHeaders 鑾峰彇涓嬭浇瀵硅薄鐨勮嚜瀹氫箟HTTP澶?	GetDownloadHeaders() map[string]string
	// GetDownloadObjects 鑾峰彇璇ヤ换鍔″綋鍓嶉渶瑕佷笅杞界殑瀵硅薄鍒楄〃
	GetDownloadObjects() ([]*model.DownloadObject, error)
	// UpdateStatus 鏇存柊涓嬭浇瀵硅薄鐨勭姸鎬?	UpdateStatus(obj *model.DownloadObject, status string, err error) error
	// Concurrency 骞跺彂鏁?	Concurrency() int
	// SetConcurrency 璁剧疆骞跺彂鏁?	SetConcurrency(int) error
	// RefreshInterval 鍒锋柊鏃堕棿
	RefreshInterval() int
	// SetRefreshInterval 璁剧疆鍒锋柊鏃堕棿
	SetRefreshInterval(int) error
	// Start 寮€濮嬩换鍔?	Start() error
	// ResolveObject 瑙ｆ瀽瀵硅薄璇︽儏锛屽～鍏?Extra["files"]锛堜富瑕佷笅杞介」鍒楄〃锛夈€?	// 鏃犻渶 resolve 鐨?task锛堝 urllist锛夌洿鎺ヨ繑鍥?nil銆?	// ctx 鐢ㄤ簬瓒呮椂鎺у埗銆?	ResolveObject(ctx context.Context, obj *model.DownloadObject) error
	// Close 鍏抽棴浠诲姟锛屾墽琛屾竻鐞嗘垨鎸佷箙鍖栨搷浣?	Close() error
}

type FailedTaskMarker interface {
	// MarkAsFailed 鏍囪浠诲姟涓哄け璐ョ姸鎬?	MarkAsFailed(obj *model.DownloadObject, err error)
}

// Downloader 瀹氫箟涓嬭浇鍣ㄧ殑琛屼负
type Downloader interface {
	// Download 鎵ц涓嬭浇
	Download(obj *model.DownloadObject, headers map[string]string) error
	// Name 杩斿洖涓嬭浇鍣ㄥ悕绉?	Name() string
}

// ContextInjecter 琛ㄧず鏀寔涓婁笅鏂囨敞鍏ョ殑涓嬭浇鍣ㄣ€?type ContextInjecter interface {
	SetContext(ctx context.Context)
}

// DomainLimiter 琛ㄧず鏀寔鍩熷悕骞跺彂闄愬埗鐨勪笅杞藉櫒銆?type DomainLimiter interface {
	ApplyDomainLimits(limits map[string]int)
}

// MetricsProvider 琛ㄧず鏀寔鏆撮湶涓嬭浇鎸囨爣鐨勪笅杞藉櫒銆?type MetricsProvider interface {
	MetricsRegistry() any // returns *download.MetricRegistry or similar
}

// SharedRegistry 鐢ㄤ簬璺ㄤ换鍔″叡浜熀浜?URL 鐨勫璞＄姸鎬?type SharedRegistry interface {
	Get(url string) (*model.DownloadObject, error)
	Update(obj *model.DownloadObject) error
	Delete(url string) error
}

// SharedRegistrySetter 浠诲姟鍙疄鐜拌鎺ュ彛浠ユ帴鏀跺叡浜敞鍐岃〃
type SharedRegistrySetter interface {
	SetSharedRegistry(reg SharedRegistry)
}

// TaskStatusGuarder 浠诲姟鍙疄鐜拌鎺ュ彛浠ユ敮鎸佸師瀛愬寲鐨勭姸鎬佸畧鍗洿鏂般€?// SetStatusUnlessCancelled 鍦?b.mu 淇濇姢涓嬫鏌ュ璞℃湭琚彇娑堝悗鏇存柊鐘舵€侊紝
// 鐢ㄤ簬瑙ｅ喅 processResolve 涓?CancelObject 涔嬮棿鐨?TOCTOU 绔炰簤銆?// 杩斿洖 true 琛ㄧず鐘舵€佸凡鏇存柊锛宖alse 琛ㄧず瀵硅薄宸茶鍙栨秷锛堣烦杩囨洿鏂帮級銆?type TaskStatusGuarder interface {
	SetStatusUnlessCancelled(obj *model.DownloadObject, status string, err error) bool
}

// EventType 瀹氫箟浜嬩欢绫诲瀷
type EventType string

const (
	EventTaskUpdate         EventType = "task_update"          // 浠诲姟缁熻鏇存柊 (鍗曚换鍔?
	EventTaskListChange     EventType = "task_list_change"     // 浠诲姟鍒楄〃鍙樺姩 (娣诲姞/鍒犻櫎浠诲姟)
	EventObjectUpdate       EventType = "object_update"        // 瀵硅薄鐘舵€?杩涘害鏇存柊
	EventSharedObjectUpdate EventType = "shared_object_update" // 鍏变韩瀵硅薄鐘舵€佹洿鏂?	EventProgressBatch      EventType = "progress_batch"       // 鎵归噺杩涘害骞挎挱
)

// Event 绯荤粺浜嬩欢
type Event struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

// EventBus 瀹氫箟浜嬩欢鎬荤嚎琛屼负
type EventBus interface {
	Subscribe() <-chan Event
	Unsubscribe(ch <-chan Event)
}
