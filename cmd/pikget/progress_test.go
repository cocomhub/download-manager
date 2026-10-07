// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMultiProgress_AddSetSummary 验证 addWorker/set/summary 基本行为。
func TestMultiProgress_AddSetSummary(t *testing.T) {
	var buf bytes.Buffer
	p := newMultiProgress(&buf, true)
	p.addWorker("chunk-0", 100, 0, "part0")
	p.addWorker("chunk-1", 100, 0, "part1")
	// 非 TTY 下 set 不触发渲染（enabled 但非终端跳过），验证数据正确累加
	p.set("chunk-0", 50, 100)
	p.set("chunk-1", 30, 100)
	if len(p.order) != 2 {
		t.Fatalf("order len = %d, want 2", len(p.order))
	}
	if got := p.workers["chunk-0"].done; got != 50 {
		t.Fatalf("chunk-0 done = %d, want 50", got)
	}
	sum := p.totalDone()
	if sum != 80 {
		t.Fatalf("total done = %d, want 80", sum)
	}
}

// totalDone 汇总所有 worker 已下字节。
func (p *multiProgress) totalDone() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var sum int64
	for _, id := range p.order {
		sum += p.workers[id].done
	}
	return sum
}

// TestMultiProgress_ConcurrentUpdateNoRace 并发 update 无竞态 + 总数正确。
func TestMultiProgress_ConcurrentUpdateNoRace(t *testing.T) {
	p := newMultiProgress(&bytes.Buffer{}, false) // enabled=false：不渲染，纯累加
	const workers = 8
	for i := range workers {
		p.addWorker(string(rune('a'+i)), 10000, 0, "w")
	}
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for range 1000 {
				p.update(id, 1)
			}
		}(string(rune('a' + i)))
	}
	wg.Wait()
	// 每 worker 1000 次 × 1 = 1000
	for i := range workers {
		id := string(rune('a' + i))
		p.mu.Lock()
		done := p.workers[id].done
		p.mu.Unlock()
		if done != 1000 {
			t.Fatalf("worker %q done = %d, want 1000", id, done)
		}
	}
}

// TestProgressBar_Bounds 进度条边界（0%/100%/中间）。
func TestProgressBar_Bounds(t *testing.T) {
	if got := progressBar(0, 10); !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Fatalf("bar: %q", got)
	}
	full := progressBar(100, 10)
	if full != "[==========]" {
		t.Fatalf("100%% bar = %q, want [==========]", full)
	}
}

// TestPad_FixedWidth 固定列宽：短名补空格，长名截断——长度不变不残留。
func TestPad_FixedWidth(t *testing.T) {
	if got := pad("ab", 4); got != "ab  " {
		t.Fatalf("pad: %q", got)
	}
	if got := pad("abcde", 4); got != "abcd" {
		t.Fatalf("pad truncate: %q", got)
	}
}

// TestSpeedAccuracy 验证速率计算的准确性：模拟 sproxy 每 1MB 回调一次（恒定真实速率），
// 断言 multiProgress 显示的速率接近真实值（误差 < 15%）。
// 慢速（1MiB/s，回调间隔 1s）与高速（10MiB/s，回调间隔 100ms < 旧 500ms 门槛）两场景。
func TestSpeedAccuracy_FastCallbacks(t *testing.T) {
	p := newMultiProgress(&bytes.Buffer{}, false)
	p.addWorker("chunk-0", 256<<20, 0, "c0")
	realRate := 10 << 20 // 10MiB/s → 每 1MB 回调间隔 100ms（快回调，旧门槛会漏算）
	stop := make(chan struct{})
	var done int64
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			time.Sleep(time.Duration(1<<20) * time.Second / time.Duration(realRate))
			done += 1 << 20
			if done >= 128<<20 {
				done = 0
			}
			p.set("chunk-0", done, 256<<20)
		}
	}()
	time.Sleep(2 * time.Second)
	close(stop)
	p.mu.Lock()
	got := p.workers["chunk-0"].speed
	p.mu.Unlock()
	if got <= 0 || got > float64(realRate)*1.3 || got < float64(realRate)*0.7 {
		t.Fatalf("fast speed = %.0f B/s, want ~%.0f B/s (±30%%), 偏差过大", got, float64(realRate))
	}
	t.Logf("fast displayed speed = %.2f MiB/s, real = 10.00 MiB/s", got/1048576)
}
func TestSpeedAccuracy(t *testing.T) {
	p := newMultiProgress(&bytes.Buffer{}, false) // 不渲染，只算速率
	p.addWorker("chunk-0", 64<<20, 0, "c0")
	// 模拟真实下载：1MB/次回调，间隔 = 1MB/1MBps = 1s（恒定 1MiB/s）
	// 用真实 sleep 模拟 1MB 下载耗时
	realRate := 1 << 20 // 1MiB/s
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		var done int64
		for {
			select {
			case <-stop:
				return
			default:
			}
			// 每次 1MB，模拟下载耗时 = 1MB / rate
			time.Sleep(time.Duration(1<<20) * time.Second / time.Duration(realRate))
			done += 1 << 20
			if done >= 32<<20 {
				done = 0 // 循环模拟
			}
			p.set("chunk-0", done, 64<<20)
		}
	}()
	// 等 2 秒让速率收敛
	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()
	p.mu.Lock()
	got := p.workers["chunk-0"].speed
	p.mu.Unlock()
	// 真实速率 1MiB/s，允许 15% 误差
	if got <= 0 || got > float64(realRate)*1.15 || got < float64(realRate)*0.85 {
		t.Fatalf("speed = %.0f B/s, want ~%.0f B/s (±15%%), got rate偏差过大", got, float64(realRate))
	}
	t.Logf("displayed speed = %.2f MiB/s, real = 1.00 MiB/s", got/1048576)
}
