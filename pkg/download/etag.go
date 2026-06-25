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
	if err != nil || fi.Size() == 0 {
		return ActionDownload
	}

	localChecksum, ckErr := checksumFunc(localPath)

	// Checksum 计算失败但有 ETag 记录 -> 尝试续传
	if ckErr != nil && prevETag != "" {
		return ActionResume
	}

	return resolveWithChecksum(prevETag, prevChecksum, localChecksum, ckErr == nil)
}

// resolveWithChecksum 在有文件且 checksum 可能可用的前提下，
// 根据 ETag/checksum 记录与本地 checksum 决定下载行为。
func resolveWithChecksum(prevETag, prevChecksum, localChecksum string, checksumOK bool) DownloadAction {
	if !checksumOK {
		// Checksum 失败且无 prevChecksum 记录 -> 全新下载
		// 有 prevChecksum 记录但无法校验 -> 重新下载
		if prevChecksum == "" {
			return ActionDownload
		}
		return ActionReDownload
	}

	// ETag 有记录但无 checksum 记录 -> 触发条件请求（If-None-Match）
	if prevETag != "" && prevChecksum == "" {
		return ActionDownload
	}

	// Checksum 匹配（无论是否有 ETag）-> 文件完整
	if prevChecksum != "" && localChecksum == prevChecksum {
		return ActionSkip
	}

	// 无 ETag、无 checksum 记录 -> 全新下载
	if prevETag == "" && prevChecksum == "" {
		return ActionDownload
	}

	// ETag 存在但 checksum 不匹配 -> 文件已变更或损坏
	return ActionReDownload
}
