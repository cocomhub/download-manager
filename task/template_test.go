// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTemplateTaskGoSkeleton 验证 task/TEMPLATE/task.go.tmpl 包含新任务接入的关键结构。
// 变异验证：删除任意关键片段（如 SetScanner 组装）后本测试应红。
func TestTemplateTaskGoSkeleton(t *testing.T) {
	t.Helper()
	data := readTemplateFile(t, "task.go.tmpl")

	required := []string{
		"task.Register(TaskType",                           // 工厂注册
		"func NewTask(cfg *config.Task",                    // 构造函数
		"task.NewPagingScanner(bt, adapter)",               // PagingScanner 组装
		"bt.SetScanner(scanner)",                           // 扫描器注入
		"bt.SetSelf(t)",                                    // 自引用注入
		"func (t *Task) Type() string",                     // 类型标识
		"func (t *Task) Scrape(ctx context.Context) error", // 抓取委托
	}
	for _, want := range required {
		if !strings.Contains(data, want) {
			t.Errorf("task.go.tmpl 缺少关键片段 %q", want)
		}
	}
}

// TestTemplateAdapterSkeleton 验证 task/TEMPLATE/adapter.go.tmpl 覆盖 SiteAdapter 全部方法。
// 变异验证：删任一方法声明后本测试应红。
func TestTemplateAdapterSkeleton(t *testing.T) {
	t.Helper()
	data := readTemplateFile(t, "adapter.go.tmpl")

	required := []string{
		"var _ task.SiteAdapter", // 编译期接口断言
		"func (a *{{TYPE}}Adapter) BuildPageURL(page int) string",
		"func (a *{{TYPE}}Adapter) RunScraper(url string) (string, error)",
		"func (a *{{TYPE}}Adapter) ParseTotalPages(html string) int",
		"func (a *{{TYPE}}Adapter) ParsePage(html string) (any, error)",
		"func (a *{{TYPE}}Adapter) ItemsToURLs(items any) []string",
		"func (a *{{TYPE}}Adapter) BuildObject(items any, index int) (*model.DownloadObject, error)",
		"a.t.GetCachedObject", // 缓存优先示范
	}
	for _, want := range required {
		if !strings.Contains(data, want) {
			t.Errorf("adapter.go.tmpl 缺少关键片段 %q", want)
		}
	}
}

// TestTemplatePlaceholders 验证模板占位符与脚手架替换一致（{{TYPE}}/{{LABEL}}）。
func TestTemplatePlaceholders(t *testing.T) {
	t.Helper()
	taskData := readTemplateFile(t, "task.go.tmpl")
	for _, ph := range []string{"{{TYPE}}", "{{LABEL}}"} {
		if !strings.Contains(taskData, ph) {
			t.Errorf("task.go.tmpl 缺少 %s 占位符", ph)
		}
	}
	adapterData := readTemplateFile(t, "adapter.go.tmpl")
	if !strings.Contains(adapterData, "{{TYPE}}") {
		t.Error("adapter.go.tmpl 缺少 {{TYPE}} 占位符")
	}
}

// readTemplateFile 读取 TEMPLATE 目录下的模板文件（相对本文件位置）。
func readTemplateFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("TEMPLATE", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s: %v", path, err)
	}
	return string(data)
}
