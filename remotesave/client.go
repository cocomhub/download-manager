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
