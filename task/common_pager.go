// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

type PageFuncs struct {
	BuildPageURL    func(page int) string
	RunScraper      func(url string) (string, error)
	ParseHomePage   func(html string) (any, error)
	ParseTotalPages func(html string) int
	ProcessItems    func(items any) (newItems []any, allKnown bool)
}

type CommonPager struct {
	funcs PageFuncs
}

func NewCommonPager(funcs PageFuncs) *CommonPager {
	return &CommonPager{funcs: funcs}
}

func (p *CommonPager) RefreshLatest() ([]any, error) {
	page := 1
	maxPages := -1
	var newObjects []any
	for {
		url := p.funcs.BuildPageURL(page)
		html, err := p.funcs.RunScraper(url)
		if err != nil {
			return newObjects, err
		}
		if maxPages == -1 {
			maxPages = p.funcs.ParseTotalPages(html)
			if maxPages <= 0 {
				maxPages = 1
			}
		}
		items, err := p.funcs.ParseHomePage(html)
		if err != nil {
			return newObjects, err
		}
		if items == nil {
			break
		}
		pageNew, allKnown := p.funcs.ProcessItems(items)
		if len(pageNew) > 0 {
			newObjects = append(newObjects, pageNew...)
		}
		if allKnown {
			break
		}
		page++
		if page > maxPages {
			break
		}
	}
	return newObjects, nil
}
