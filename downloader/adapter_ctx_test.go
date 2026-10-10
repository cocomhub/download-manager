// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"testing"
)

// TestDownloaderAdapter_PerURLCtxIsolation 验证默认适配器也按 URL 隔离上下文：
// 兄弟下载（共享实例）取消自己的 ctx 不得中断本对象在途下载。
func TestDownloaderAdapter_PerURLCtxIsolation(t *testing.T) {
	t.Parallel()
	a := &DownloaderAdapter{}
	ctxA, cancelA := context.WithCancel(t.Context())
	ctxB, cancelB := context.WithCancel(t.Context())
	defer cancelB()

	// 模拟：兄弟下载已把共享字段覆盖为 A 的 ctx，而 B 按 URL 注入自己的 ctx
	a.SetContext(ctxA)
	a.SetContextFor("http://b", ctxB)
	cancelA()

	if err := a.getCtxFor("http://b").Err(); err != nil {
		t.Fatalf("B 应使用按 URL 注入的 ctx，不应被 A 的取消影响: %v", err)
	}
	if err := a.getCtxFor("http://c").Err(); err == nil {
		t.Fatal("未按 URL 注入的 URL 应回落进程级 ctx（此时已取消）")
	}
	a.clearContextFor("http://b")
	if err := a.getCtxFor("http://b").Err(); err == nil {
		t.Fatal("clearContextFor 后应回落进程级 ctx")
	}
}

// TestDownloaderAdapter_SetContextFor_EmptyURL 空 URL 不写入（避免脏键）。
func TestDownloaderAdapter_SetContextFor_EmptyURL(t *testing.T) {
	t.Parallel()
	a := &DownloaderAdapter{}
	a.SetContextFor("", t.Context())
	if len(a.urlCtx) != 0 {
		t.Fatalf("空 URL 不应写入 urlCtx, got %d", len(a.urlCtx))
	}
}
