// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"os"
)

// DownloadAction 决定本次下载操作的类型。
type DownloadAction int

const (
	// ActionDownload 全新下载（文件不存在或无法验证）。
	ActionDownload DownloadAction = iota
	// ActionResume 断点续传（文件存在不完整，无 checksum 但有 ETag）。
	ActionResume
	// ActionSkip 跳过下载（ETag + checksum 一致，文件完整）。
	ActionSkip
	// ActionReDownload 重新下载（ETag 不一致或 checksum 不匹配）。
	ActionReDownload
)

// FileStatFunc 文件状态查询接口，可由 os.Stat 或 mock 实现。
type FileStatFunc func(path string) (os.FileInfo, error)

// FileChecksumFunc 文件校验和计算接口，可由 ComputeFileMD5 或 mock 实现。
type FileChecksumFunc func(path string) (string, error)

// ResolveAction 根据文件是否存在、ETag 匹配状态和 checksum 校验结果，
// 决定本次下载应采取何种操作（全新下载/续传/跳过/重新下载）。
//
// 决策矩阵：
//
//	文件存在?	ETag 匹配?	Checksum 匹配?	行为
//	否				—				—				ActionDownload
//	完整			一致			一致				ActionSkip
//	完整			一致			不存在/不一致		ActionReDownload(文件损坏)
//	不完整		一致				—				ActionResume
//	完整			不一致			—				ActionReDownload
//	完整			无记录(旧数据)	存在且一致		ActionSkip(兼容旧数据)
//	完整			无记录(旧数据)	不存在			ActionDownload
func ResolveAction(localPath string, prevETag, prevChecksum string, statFunc FileStatFunc, checksumFunc FileChecksumFunc) DownloadAction {
	fi, err := statFunc(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionDownload
		}
		// 无法 stat 视为不存在，安全走全新下载
		return ActionDownload
	}

	fileSize := fi.Size()
	// 文件大小为 0 视为不存在
	if fileSize == 0 {
		return ActionDownload
	}

	// 文件不完整（无 checksum 可校验，说明上次未完成下载）
	// 如果 prevETag != ""，尝试续传，否则重新下载
	localChecksum, ckErr := checksumFunc(localPath)
	if ckErr != nil && prevETag != "" {
		return ActionResume
	}

	// 有 ETag 记录但无 checksum 记录，触发条件请求（If-None-Match）
	// 如果服务端返回 304，表示文件未变更，跳过下载
	if prevETag != "" && prevChecksum == "" {
		// 文件完好但无上次 checksum 记录，走条件请求让服务端决定
		return ActionDownload
	}

	// 兼容旧数据：无 ETag 但有 checksum 且匹配文件
	if prevETag == "" && prevChecksum != "" && localChecksum == prevChecksum {
		return ActionSkip
	}

	// 兼容旧数据：无 ETag 且无 checksum 记录
	if prevETag == "" && prevChecksum == "" {
		return ActionDownload
	}

	// ETag 匹配且 checksum 匹配 → 跳过
	if prevETag != "" && localChecksum == prevChecksum {
		return ActionSkip
	}

	// ETag 匹配但 checksum 不匹配 → 文件损坏，需要重下
	if prevETag != "" && localChecksum != prevChecksum {
		return ActionReDownload
	}

	// ETag 不匹配
	return ActionReDownload
}
