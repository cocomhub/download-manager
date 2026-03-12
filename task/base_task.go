// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type PagingCap interface {
	SetPager(*CommonPager)
}

type RefreshingCap interface {
	SetRefresher(*CommonRefresher)
}

type PathStrategyCap interface {
	SetPathStrategy(core.PathStrategy)
}

type HeadersCap interface {
	SetHeaders(map[string]string)
}

type BaseTaskImpl struct {
	id           string
	saveDir      string
	store        core.Storage
	shared       core.SharedRegistry
	mu           sync.Mutex
	refresher    *CommonRefresher
	pager        *CommonPager
	pathStrategy core.PathStrategy
	headers      map[string]string
}

func (b *BaseTaskImpl) ID() string {
	return b.id
}

func (b *BaseTaskImpl) GetDownloadHeaders() map[string]string {
	if b.headers == nil {
		return map[string]string{}
	}
	return b.headers
}

func (b *BaseTaskImpl) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	b.mu.Lock()
	obj.Status = status
	b.mu.Unlock()
	var e error
	if b.store != nil {
		e = b.store.Update(obj)
	}
	if b.shared != nil {
		_ = b.shared.Update(obj)
	}
	return e
}

func (b *BaseTaskImpl) Type() string {
	return "base"
}

func (b *BaseTaskImpl) Close() error {
	if b.refresher != nil {
		b.refresher.Stop()
	}
	return nil
}

func (b *BaseTaskImpl) SetSharedRegistry(reg core.SharedRegistry) {
	b.shared = reg
}

func (b *BaseTaskImpl) SetDownloader(d core.Downloader) {
}

func (b *BaseTaskImpl) SetPathStrategy(ps core.PathStrategy) {
	b.pathStrategy = ps
}

func (b *BaseTaskImpl) SetPager(p *CommonPager) {
	b.pager = p
}

func (b *BaseTaskImpl) SetRefresher(r *CommonRefresher) {
	b.refresher = r
}

func (b *BaseTaskImpl) SetHeaders(h map[string]string) {
	b.headers = h
}
