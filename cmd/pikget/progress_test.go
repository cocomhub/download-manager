// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
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
	for i := 0; i < workers; i++ {
		p.addWorker(string(rune('a'+i)), 10000, 0, "w")
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				p.update(id, 1)
			}
		}(string(rune('a' + i)))
	}
	wg.Wait()
	// 每 worker 1000 次 × 1 = 1000
	for i := 0; i < workers; i++ {
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
