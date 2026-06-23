// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
)

// compile-time interface checks
var (
	_ core.Downloader                 = (*DownloaderAdapter)(nil)
	_ core.ContextInjecter      = (*DownloaderAdapter)(nil)
	_ core.DomainLimiter = (*DownloaderAdapter)(nil)
)

// DownloaderAdapter 灏?pkg/download.Downloader 鍖呰涓?core.Downloader銆?// 澶勭悊 model.DownloadObject 鈫?download.Request 鐨勬槧灏勶紝鍖呮嫭杩涘害杞崲銆?type DownloaderAdapter struct {
	mu              sync.Mutex
	dl              *download.Downloader
	dlCtx           context.Context
	transport       download.Transport
	cancels         sync.Map // map[string]context.CancelFunc
	metrics         *download.MetricRegistry
	metadataFlusher func() // 鐢?Manager 璁剧疆锛孫nMetadata 瑙﹀彂鍚庤皟鐢ㄤ互绔嬪嵆鎸佷箙鍖?}

// NewDownloaderAdapter 鍒涘缓閫傞厤鍣ㄣ€?func NewDownloaderAdapter(dl *download.Downloader) *DownloaderAdapter {
	return &DownloaderAdapter{
		dl: dl,
	}
}

// SetMetadataFlusher 璁剧疆涓€涓洖璋冿紝鍦ㄦ瘡娆?OnMetadata 鍐欏叆 obj.Metadata 鍚庤皟鐢紝
// 鐢ㄤ簬绔嬪嵆鎸佷箙鍖栵紙閬垮厤 crash 绐楀彛锛夈€?// 蹇呴』鍦?Download 鍓嶈皟鐢紝涓嶅彲骞跺彂璋冪敤銆?func (a *DownloaderAdapter) SetMetadataFlusher(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.metadataFlusher = fn
}

// getMetadataFlusher 杩斿洖 metadataFlusher锛堢嚎绋嬪畨鍏紝涓?SetMetadataFlusher 浜掓枼锛夈€?func (a *DownloaderAdapter) getMetadataFlusher() func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.metadataFlusher
}

// Name 杩斿洖閫傞厤鍣ㄥ悕绉帮紙淇濇寔涓庢棫 NativeHTTPDownloader 鍏煎锛夈€?func (a *DownloaderAdapter) Name() string { return "native_http" }

// SetContext 璁剧疆涓嬭浇涓婁笅鏂囷紙鏇夸唬鏃х殑 NativeHTTPDownloader.SetContext锛夈€?func (a *DownloaderAdapter) SetContext(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dlCtx = ctx
}

// ApplyDomainLimits 璁剧疆鍩熷悕骞跺彂闄愬埗锛堥€氳繃 StdlibTransport锛夈€?func (a *DownloaderAdapter) ApplyDomainLimits(limits map[string]int) {
	if tr, ok := a.transport.(*download.StdlibTransport); ok {
		tr.SetDomainLimits(limits)
	}
}

// Cancel 灏濊瘯鍙栨秷姝ｅ湪杩涜鐨勪笅杞姐€備娇鐢?per-URL cancel func 瀹炵幇銆?func (a *DownloaderAdapter) Cancel(url string) error {
	if v, ok := a.cancels.LoadAndDelete(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
	}
	return nil
}

// MetricsRegistry returns the download MetricRegistry (implements core.MetricsProvider).
func (a *DownloaderAdapter) MetricsRegistry() any {
	return a.metrics
}

// getCtx 杩斿洖褰撳墠涓婁笅鏂囷紙绾跨▼瀹夊叏锛夈€?func (a *DownloaderAdapter) getCtx() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dlCtx != nil {
		return a.dlCtx
	}
	return context.Background()
}

// copyMetadata 澶嶅埗 map锛屼娇 req.Metadata 涓嶇洿鎺ュ紩鐢?obj.Metadata锛?// 浣嗕繚鐣欏凡鏈?ETag/checksum 渚?http_extractor 鐨勬潯浠惰姹傛鏌ャ€?func copyMetadata(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

// Download 瀹炵幇 core.Downloader 鎺ュ彛銆?// 灏?model.DownloadObject + headers 鏄犲皠涓?download.Request 骞舵墽琛屼笅杞姐€?func (a *DownloaderAdapter) Download(obj *model.DownloadObject, headers map[string]string) error {
	ctx := a.getCtx()

	if headers == nil {
		headers = make(map[string]string)
	}

	// 澶勭悊澶嶅悎涓嬭浇锛氭鏌?Extra["files"]
	// Must lock the object to synchronize with concurrent writes
	// from applySharedState (task/base_task.go).
	obj.RLock()
	filesVal, hasFiles := obj.Extra["files"]
	obj.RUnlock()
	if hasFiles {
		return a.downloadComposite(ctx, obj, headers, filesVal)
	}

	// 鏍囧噯鍗曟枃浠朵笅杞?	req := &download.Request{
		URL:           obj.URL,
		SavePath:      obj.SavePath,
		Headers:       headers,
		Metadata:      func() map[string]string { obj.RLock(); defer obj.RUnlock(); return copyMetadata(obj.Metadata) }(),
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			obj.SetProgress(int(progress))
		},
		OnMetadata: func(key, value string) {
			obj.Lock()
			if obj.Metadata == nil {
				obj.Metadata = make(map[string]string)
			}
			obj.Metadata[key] = value
			obj.Unlock()
			if a.getMetadataFlusher() != nil {
				a.getMetadataFlusher()()
			}
		},
	}

	// 鍒涘缓 per-URL 鍙彇娑堢殑 context
	dlCtx, dlCancel := context.WithCancel(ctx)
	defer a.cancels.Delete(obj.URL)
	a.cancels.Store(obj.URL, dlCancel)

	err := a.dl.Download(dlCtx, req)
	if err != nil {
		return fmt.Errorf("adapter: download failed: %w", err)
	}

	// 灏?DownloadResult 鏄惧紡鍐欏叆 obj.Metadata锛堝姞閿佷繚鎶わ級
	obj.Lock()
	if r := req.Result; r != nil {
		if r.StatusCode > 0 {
			obj.Metadata["status_code"] = strconv.Itoa(r.StatusCode)
		}
		if r.ContentLength > 0 {
			obj.Metadata["content_length"] = strconv.FormatInt(r.ContentLength, 10)
		}
		if r.TotalSize > 0 {
			obj.Metadata["total_size"] = strconv.FormatInt(r.TotalSize, 10)
		}
		if r.MD5Base64 != "" {
			obj.Metadata["md5_base64"] = r.MD5Base64
		}
		if r.MD5Hex != "" {
			obj.Metadata["md5_hex"] = r.MD5Hex
		}
		if r.ModTime != "" {
			obj.Metadata["mod_time"] = r.ModTime
		}
		// Compatibility shim: set status metadata for consumers not yet migrated
		// to the pkg/download result model. Remove when dlcore is fully removed.
		obj.Metadata["status"] = "completed"
	}
	obj.Unlock()

	obj.SetProgress(100)
	return nil
}

// downloadComposite 澶勭悊澶嶅悎涓嬭浇锛圗xtra["files"]锛夈€?// 姣忎釜瀛愭枃浠舵寜 type 鍓嶇紑鐙珛淇濆瓨 ETag/checksum锛堝 cover_etag銆乿ideo_checksum 绛夛級锛?// 骞跺湪涓嬭浇鍓嶄紶鍏ュ凡鏈夌殑鍓嶇紑鍏冩暟鎹互渚挎柇鐐圭画浼犲拰鏉′欢璇锋眰銆?func (a *DownloaderAdapter) downloadComposite(ctx context.Context, obj *model.DownloadObject, headers map[string]string, filesVal any) error {
	fileList, err := parseCompositeFiles(filesVal)
	if err != nil {
		return err
	}
	// parseCompositeFiles already handles empty list 鈥?returns ErrCompositeEmpty

	slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
	for _, fileMap := range fileList {
		subURL := fileMap["url"]
		subPath := fileMap["path"]
		fType := fileMap[model.MetadataKeyType]

		if subURL == "" || subPath == "" {
			continue
		}

		// 纭繚瀛愭枃浠剁洰褰曞瓨鍦?		dir := filepath.Dir(subPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("adapter: failed to create directory for composite file: %w", err)
		}

		// 璺熻釜杩涘害锛氫粎 video 绫诲瀷鎴栧彧鏈変竴涓枃浠舵椂
		trackProgress := (fType == "video" || len(fileList) == 1)

		// 涓哄瓙鏂囦欢鏋勫缓甯﹀墠缂€鐨?metadata锛屼紶鍏ュ凡鏈夌殑 ETag/checksum 渚涙潯浠惰姹傛鏌?		subMeta := make(map[string]string)
		prefix := fType + "_"
		obj.RLock()
		if v, ok := obj.Metadata[prefix+"etag"]; ok {
			subMeta["etag"] = v
		}
		if v, ok := obj.Metadata[prefix+"checksum"]; ok {
			subMeta["checksum"] = v
		}
		obj.RUnlock()

		subReq := &download.Request{
			URL:           subURL,
			SavePath:      subPath,
			Headers:       headers,
			Metadata:      subMeta,
			TrackProgress: trackProgress,
			OnProgress: func(progress float64, downloaded, total int64) {
				obj.SetProgress(int(progress))
			},
			OnMetadata: func(key, value string) {
				// 鎸?type 鍓嶇紑鍐欏叆 obj.Metadata锛堝 cover_etag銆乿ideo_checksum锛?				obj.Lock()
				if obj.Metadata == nil {
					obj.Metadata = make(map[string]string)
				}
				obj.Metadata[prefix+key] = value
				obj.Unlock()
				if a.getMetadataFlusher() != nil {
					a.getMetadataFlusher()()
				}
			},
		}

		if err := a.dl.Download(ctx, subReq); err != nil {
			return fmt.Errorf("adapter: sub-download failed (%s): %w", subURL, err)
		}
	}

	obj.SetProgress(100)
	return nil
}
