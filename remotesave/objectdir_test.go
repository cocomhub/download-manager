// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"testing"

	"github.com/cocomhub/download-manager/model"
)

func TestObjectKey_Priority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		obj  *model.DownloadObject
		want string
	}{
		{"code", &model.DownloadObject{Metadata: map[string]string{"code": "ABC-001"}}, "ABC-001"},
		{"content_group", &model.DownloadObject{Metadata: map[string]string{"content_group": "XYZ-002"}}, "XYZ-002"},
		{"id", &model.DownloadObject{ID: 407014}, "407014"},
		{"title", &model.DownloadObject{Metadata: map[string]string{"title": "标题"}}, "标题"},
	}
	for _, c := range cases {
		if got := ObjectKey(c.obj); got != c.want {
			t.Errorf("%s: ObjectKey=%q want %q", c.name, got, c.want)
		}
	}
}

func TestObjectRemotePaths_DisabledKeepsRel(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{Metadata: map[string]string{"code": "ABC-001"}}
	got := objectRemotePaths(obj, nil, []string{"sub/x.mp4"}, TaskConfig{Dir: "hanime/2.5D"})
	if got["sub/x.mp4"] != "hanime/2.5D/sub/x.mp4" {
		t.Fatalf("got %q", got["sub/x.mp4"])
	}
}

func TestObjectRemotePaths_ObjectDirsAndCollision(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{Metadata: map[string]string{"code": "ABC-001"}}
	cfg := TaskConfig{Dir: "hanime/2.5D", ObjectDirs: true}
	got := objectRemotePaths(obj, nil, []string{"a/x.mp4", "b/x.mp4"}, cfg)
	if got["a/x.mp4"] != "hanime/2.5D/ABC-001/x.mp4" {
		t.Fatalf("first=%q", got["a/x.mp4"])
	}
	second := got["b/x.mp4"]
	if second == "hanime/2.5D/ABC-001/x.mp4" || len(second) <= len("hanime/2.5D/ABC-001/x.mp4") {
		t.Fatalf("collision 未消歧: %q", second)
	}
}
