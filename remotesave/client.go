// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/config"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

// FileInfo 是远端文件的验证信息（size + sproxy 侧 sha256）。
type FileInfo struct {
	Size     int64
	Checksum string
	ModTime  int64
}

// MetaInfo 是 sproxy 保存的原始文件时间与 meta。
type MetaInfo struct {
	MTime string
	CTime string
	Extra map[string]any
}

// Client 是 remote-save 对 sproxy 的最小依赖面（便于构造注入 fake）。
type Client interface {
	Stat(ctx context.Context, remote string) (*FileInfo, error)
	Upload(ctx context.Context, local, remote string) (string, error)
	UpdateMeta(ctx context.Context, remote string, meta map[string]any) error
	GetMeta(ctx context.Context, remote string) (MetaInfo, error)
	Delete(ctx context.Context, remote string) error
	Rename(ctx context.Context, from, to string) error
}

type sproxyClient struct {
	fc *sproxyclient.FileClient
}

// NewSproxyClient 按全局 sproxy 配置构造客户端（卷上下文固定为 volume）。
func NewSproxyClient(sp config.Sproxy, volume string, insecureTLS bool) (Client, error) {
	if sp.APIURL == "" {
		return nil, fmt.Errorf("sproxy.api_url 未配置")
	}
	base := strings.TrimSuffix(strings.TrimRight(sp.APIURL, "/"), "/api/cloud/download")
	opts := []sproxyclient.Option{
		sproxyclient.WithVolume(volume),
		sproxyclient.WithTimeout(6 * time.Hour),
	}
	switch {
	case sp.AccessKey != "" && sp.AccessKeySecret != "" && sp.AccessKeyID != "":
		opts = append(opts, sproxyclient.WithAccessKey(sp.AccessKey, sp.AccessKeySecret), sproxyclient.WithAccessKeyID(sp.AccessKeyID))
	case sp.AccessKey != "" || sp.AccessKeySecret != "" || sp.AccessKeyID != "":
		return nil, fmt.Errorf("sproxy SproxySig 三件套必须同时配置（access_key/access_key_secret/access_key_id）")
	case sp.APIToken != "":
		opts = append(opts, sproxyclient.WithBearerToken(sp.APIToken))
	default:
		return nil, fmt.Errorf("sproxy 未配置任何认证凭据")
	}
	if insecureTLS || sp.InsecureTLS {
		opts = append(opts, sproxyclient.WithInsecureTLS())
	}
	return &sproxyClient{fc: sproxyclient.NewFileClient(base, opts...)}, nil
}

func (c *sproxyClient) Stat(ctx context.Context, remote string) (*FileInfo, error) {
	fi, err := c.fc.Stat(ctx, remote)
	if err != nil {
		if errors.Is(err, sproxyclient.ErrNotFound) {
			return nil, sproxyclient.ErrNotFound
		}
		return nil, err
	}
	return &FileInfo{Size: fi.Size, Checksum: fi.Checksum, ModTime: fi.ModTime}, nil
}

func (c *sproxyClient) Upload(ctx context.Context, local, remote string) (string, error) {
	st, err := os.Stat(local)
	if err != nil {
		return "", err
	}
	if st.Size() > 64<<20 {
		res, uerr := c.fc.ChunkedUpload(ctx, local, remote,
			sproxyclient.WithChunkedResume(true),
			sproxyclient.WithChunkedConcurrency(4),
		)
		if uerr != nil {
			return "", uerr
		}
		return res.FileChecksum, nil
	}
	res, uerr := c.fc.Upload(ctx, local, remote)
	if uerr != nil {
		return "", uerr
	}
	return res.Checksum, nil
}

func (c *sproxyClient) UpdateMeta(ctx context.Context, remote string, meta map[string]any) error {
	return c.fc.UpdateFileMeta(ctx, remote, meta)
}

func (c *sproxyClient) GetMeta(ctx context.Context, remote string) (MetaInfo, error) {
	mi, err := c.fc.GetFileMetaInfo(ctx, remote)
	if err != nil {
		return MetaInfo{}, err
	}
	return MetaInfo{MTime: mi.MTime, CTime: mi.CTime, Extra: mi.Extra}, nil
}

func (c *sproxyClient) Delete(ctx context.Context, remote string) error {
	return c.fc.Delete(ctx, remote, "")
}

func (c *sproxyClient) Rename(ctx context.Context, from, to string) error {
	fi, err := c.fc.Stat(ctx, from)
	if err != nil {
		return err
	}
	return c.fc.Rename(ctx, from, to, fi.Checksum)
}

// deleteRemote 是幂等删除：目标已不存在视为成功（覆盖/镜像清理崩溃后的重试路径）。
func deleteRemote(ctx context.Context, client Client, remote string) error {
	err := client.Delete(ctx, remote)
	if errors.Is(err, sproxyclient.ErrNotFound) {
		return nil
	}
	return err
}

// uploadOrReuse 用于 rename：目标已存在且 checksum 相同则复用，避免“上传成功但
// manifest 未落盘”的中断场景永久失败；目标存在但内容不同则拒绝。
func uploadOrReuse(ctx context.Context, client Client, local, remote, sha string) (string, error) {
	fi, err := client.Stat(ctx, remote)
	switch {
	case err == nil && fi != nil:
		if strings.EqualFold(fi.Checksum, sha) {
			return sha, nil
		}
		return "", fmt.Errorf("目标已存在且内容不同: %s", remote)
	case err != nil && !errors.Is(err, sproxyclient.ErrNotFound):
		return "", err
	default:
		return client.Upload(ctx, local, remote)
	}
}

func renameRemoteOrReuse(ctx context.Context, client Client, from, to, checksum string) error {
	if fi, err := client.Stat(ctx, to); err == nil && fi != nil {
		if checksum != "" && strings.EqualFold(fi.Checksum, checksum) {
			return nil
		}
		return fmt.Errorf("rename: 目标已存在: %s", to)
	} else if err != nil && !errors.Is(err, sproxyclient.ErrNotFound) {
		return err
	}
	return client.Rename(ctx, from, to)
}
