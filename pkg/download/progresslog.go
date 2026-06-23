// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// ProgressReport 鏄崟娆¤繘搴﹀揩鐓х殑鍊煎璞★紝鍖呭惈璁＄畻鏍煎紡鍖栨棩蹇楄鎵€闇€鐨勫叏閮ㄥ瓧娈点€?type ProgressReport struct {
	Timestamp  time.Time     // 璇ユ鍥炶皟鐨勬椂闂存埑
	Progress   float64       // 0-100 鐨勫畬鎴愮櫨鍒嗘瘮
	Downloaded int64         // 宸蹭笅杞藉瓧鑺傛暟
	Total      int64         // 鎬诲瓧鑺傛暟锛堜繚璇?>0锛?	Speed      float64       // 鑷笂娆″洖璋冧互鏉ョ殑骞冲潎閫熷害锛坆ytes/s锛?	Elapsed    time.Duration // 鑷娆″洖璋冧互鏉ョ殑宸茬敤鏃堕棿
	ETA        time.Duration // 棰勮鍓╀綑鏃堕棿锛堝綋 Speed=0 鏃朵负 0锛?}

// ProgressLogOption 閰嶇疆 NewProgressLogCallback 鐨勮涓恒€?type ProgressLogOption func(*progressLogConfig)

type progressLogConfig struct {
	w           io.Writer
	minStep     float64
	maxInterval time.Duration
	formatter   func(io.Writer, ProgressReport)
}

func defaultConfig() *progressLogConfig {
	return &progressLogConfig{
		w:           io.Discard,
		minStep:     0.5,
		maxInterval: 10 * time.Second,
		formatter:   defaultProgressFormatter,
	}
}

// WithLogWriter 璁剧疆杩涘害鏃ュ織鐨勮緭鍑虹洰鏍囥€傞粯璁?io.Discard銆?func WithLogWriter(w io.Writer) ProgressLogOption {
	return func(c *progressLogConfig) {
		c.w = w
	}
}

// WithMinPercentStep 璁剧疆瑙﹀彂鏃ュ織鍐欏叆鐨勬渶灏忚繘搴︾櫨鍒嗘瘮鍙樺寲銆傞粯璁?0.5銆?// 璁句负 0 琛ㄧず姣忔鍥炶皟閮藉啓鍏ャ€?func WithMinPercentStep(step float64) ProgressLogOption {
	return func(c *progressLogConfig) {
		if step < 0 {
			step = 0
		}
		c.minStep = step
	}
}

// WithMaxInterval 璁剧疆涓ゆ鏃ュ織鍐欏叆涔嬮棿鐨勬渶澶ч棿闅斻€傞粯璁?10s銆?// 鍗充娇杩涘害鍙樺寲鏈揪鍒版渶灏忔闀匡紝瓒呰繃姝ら棿闅斾篃浼氬己鍒跺啓鍏ャ€?func WithMaxInterval(d time.Duration) ProgressLogOption {
	return func(c *progressLogConfig) {
		if d <= 0 {
			d = time.Second
		}
		c.maxInterval = d
	}
}

// WithProgressFormatter 璁剧疆鑷畾涔夋棩蹇楁牸寮忓寲鍑芥暟銆傞粯璁や娇鐢?defaultProgressFormatter銆?func WithProgressFormatter(fn func(io.Writer, ProgressReport)) ProgressLogOption {
	return func(c *progressLogConfig) {
		c.formatter = fn
	}
}

// NewProgressLogCallback 鍒涘缓涓€涓甫鑺傛祦鐨勮繘搴︽棩蹇楀洖璋冦€?//
// 杩斿洖鐨勫嚱鏁扮鍚嶄笌 Request.OnProgress 涓€鑷达紝鍙洿鎺ヤ娇鐢ㄦ垨閫氳繃 ComposeProgress 涓庣幇鏈夊洖璋冪粍鍚堛€?// 棣栨璋冪敤鏃犳潯浠跺啓鍏ヨ捣鐐规棩蹇楋紱鍚庣画璋冪敤鍦ㄨ繘搴﹀彉鍖栬秴杩?minStep 鎴栬窛绂讳笂娆″啓鍏ヨ秴杩?maxInterval 鏃跺啓鍏ャ€?//
// 浣跨敤鏂规硶锛?//
//	req.OnProgress = NewProgressLogCallback(
//	    WithLogWriter(logFile),
//	    WithMinPercentStep(0.5),
//	    WithMaxInterval(10*time.Second),
//	)
func NewProgressLogCallback(opts ...ProgressLogOption) func(float64, int64, int64) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	var (
		initialTime   = time.Now()
		lastTime      = initialTime
		lastProgress  = -1.0 // 纭繚绗竴娆¤皟鐢ㄤ竴瀹氬啓鍏?		lastBytes     int64
		firstCallDone bool
		mu            sync.Mutex
	)

	return func(progress float64, downloaded, total int64) {
		if cfg.w == nil {
			return
		}

		now := time.Now()

		mu.Lock()

		// 璁＄畻閫熷害锛堜繚鎶ら櫎闆讹級
		var speed float64
		elapsed := now.Sub(lastTime).Seconds()
		if elapsed > 0 {
			speed = float64(downloaded-lastBytes) / elapsed
		}

		report := ProgressReport{
			Timestamp:  now,
			Progress:   progress,
			Downloaded: downloaded,
			Total:      total,
			Speed:      speed,
			Elapsed:    now.Sub(initialTime),
		}
		if speed > 0 && total > downloaded {
			report.ETA = time.Duration(float64(total-downloaded)/speed) * time.Second
		}

		// 棣栨璋冪敤鎴栨弧瓒宠妭娴佹潯浠舵椂鍐欏叆
		deltaPct := progress - lastProgress
		timeSinceLastWrite := now.Sub(lastTime)

		if !firstCallDone || deltaPct >= cfg.minStep || timeSinceLastWrite >= cfg.maxInterval {
			mu.Unlock()
			cfg.formatter(cfg.w, report)
			mu.Lock()
			firstCallDone = true
			lastTime = now
			lastProgress = progress
			lastBytes = downloaded
		}

		mu.Unlock()
	}
}

// defaultProgressFormatter 鏄粯璁ょ殑杩涘害鏃ュ織鏍煎紡鍖栧嚱鏁般€?// 杈撳嚭鏍煎紡锛?//
//	2026-06-10T12:00:01.000000000Z Progress:  0.000%  0.00 B/s expected time: --.- s
func defaultProgressFormatter(w io.Writer, r ProgressReport) {
	// 鏃堕棿鎴?	ts := r.Timestamp.Format(time.RFC3339Nano)

	// 鐧惧垎姣旓細7 瀛楃瀹藉彸瀵归綈锛堝 "  0.000%", "100.000%"锛?	pctStr := fmt.Sprintf("%7.3f%%", r.Progress)

	// 閫熷害鍗曚綅鑷€傚簲
	speedVal := r.Speed
	unit := "B/s"
	switch {
	case speedVal >= 1<<30:
		speedVal /= 1 << 30
		unit = "GB/s"
	case speedVal >= 1<<20:
		speedVal /= 1 << 20
		unit = "MB/s"
	case speedVal >= 1<<10:
		speedVal /= 1 << 10
		unit = "KB/s"
	}

	// ETA
	var etaStr string
	if r.Speed > 0 && r.Total > r.Downloaded {
		etaSec := float64(r.Total-r.Downloaded) / r.Speed
		etaStr = fmt.Sprintf("%5.2f s", etaSec)
	} else {
		etaStr = "--.- s"
	}

	_, err := fmt.Fprintf(w, "%s Progress: %s  %.2f %s expected time: %s\n",
		ts, pctStr, speedVal, unit, etaStr)
	if err != nil {
		slog.Warn("Failed to write progress log", "error", err)
	}
}
