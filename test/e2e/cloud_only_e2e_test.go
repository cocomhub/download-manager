// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

const tinyMP4Base64 = `AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAAIZnJlZQAABDltZGF0AAACrgYF//+q3EXpvebZSLeWLNgg2SPu73gy
NjQgLSBjb3JlIDE2NSByMzIyMyAwNDgwY2IwIC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyNSAt
IGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTMgZGVibG9jaz0xOjA6MCBh
bmFseXNlPTB4MzoweDExMyBtZT1oZXggc3VibWU9NyBwc3k9MSBwc3lfcmQ9MS4wMDowLjAwIG1peGVkX3JlZj0xIG1lX3Jhbmdl
PTE2IGNocm9tYV9tZT0xIHRyZWxsaXM9MSA4eDhkY3Q9MSBjcW09MCBkZWFkem9uZT0yMSwxMSBmYXN0X3Bza2lwPTEgY2hyb21h
X3FwX29mZnNldD0tMiB0aHJlYWRzPTIgbG9va2FoZWFkX3RocmVhZHM9MSBzbGljZWRfdGhyZWFkcz0wIG5yPTAgZGVjaW1hdGU9
MSBpbnRlcmxhY2VkPTAgYmx1cmF5X2NvbXBhdD0wIGNvbnN0cmFpbmVkX2ludHJhPTAgYmZyYW1lcz0zIGJfcHlyYW1pZD0yIGJf
YWRhcHQ9MSBiX2JpYXM9MCBkaXJlY3Q9MSB3ZWlnaHRiPTEgb3Blbl9nb3A9MCB3ZWlnaHRwPTIga2V5aW50PTI1MCBrZXlpbnRf
bWluPTI1IHNjZW5lY3V0PTQwIGludHJhX3JlZnJlc2g9MCByY19sb29rYWhlYWQ9NDAgcmM9Y3JmIG1idHJlZT0xIGNyZj0yMy4w
IHFjb21wPTAuNjAgcXBtaW49MCBxcG1heD02OSBxcHN0ZXA9NCBpcF9yYXRpbz0xLjQwIGFxPTE6MS4wMACAAAAAJ2WIhAA7//7j
q/gU2FBUdEzFKP6FtGNPzxSPTYUNLTnUBLOor0B3gQAAAApBmiRsQ7/+qZ00AAAACEGeQniF/wm5AAAACAGeYXRCvww4AAAACAGe
Y2pCvww5AAAAEEGaaEmoQWiZTAh3//6pnTUAAAAKQZ6GRREsL/8JuQAAAAgBnqV0Qr8MOQAAAAgBnqdqQr8MOAAAABBBmqxJqEFs
mUwId//+qZ00AAAACkGeykUVLC//CbkAAAAIAZ7pdEK/DDgAAAAIAZ7rakK/DDgAAAAQQZrwSahBbJlMCG///qePiQAAAApBnw5F
FSwv/wm5AAAACAGfLXRCvww5AAAACAGfL2pCvww4AAAAEEGbNEmoQWyZTAhn//6eLfAAAAAKQZ9SRRUsL/8JuQAAAAgBn3F0Qr8M
OAAAAAgBn3NqQr8MOAAAABBBm3hJqEFsmUwIV//+OI3BAAAACkGflkUVLC//CbgAAAAIAZ+1dEK/DDkAAAAIAZ+3akK/DDkAAARl
bW9vdgAAAGxtdmhkAAAAAAAAAAAAAAAAAAAD6AAAA+gAAQAAAQAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAA
AAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAA5B0cmFrAAAAXHRraGQAAAADAAAAAAAAAAAAAAABAAAAAAAA
A+gAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAEAAAABAAAAAAAAkZWR0cwAAABxl
bHN0AAAAAAAAAAEAAAPoAAAEAAABAAAAAAMIbWRpYQAAACBtZGhkAAAAAAAAAAAAAAAAAAAyAAAAMgBVxAAAAAAALWhkbHIAAAAA
AAAAAHZpZGUAAAAAAAAAAAAAAABWaWRlb0hhbmRsZXIAAAACs21pbmYAAAAUdm1oZAAAAAEAAAAAAAAAAAAAACRkaW5mAAAAHGRy
ZWYAAAAAAAAAAQAAAAx1cmwgAAAAAQAAAnNzdGJsAAAAv3N0c2QAAAAAAAAAAQAAAK9hdmMxAAAAAAAAAAEAAAAAAAAAAAAAAAAA
AAAAAEAAQABIAAAASAAAAAAAAAABFExhdmM2My4xLjEwMiBsaWJ4MjY0AAAAAAAAAAAAAAAAGP//AAAANWF2Y0MBZAAK/+EAGGdk
AAqs2UQmwEQAAAMABAAAAwDIPEiWWAEABmjr48siwP34+AAAAAAQcGFzcAAAAAEAAAABAAAAFGJ0cnQAAAAAAAAhiAAAAAAAAAAY
c3R0cwAAAAAAAAABAAAAGQAAAgAAAAAUc3RzcwAAAAAAAAABAAAAAQAAANhjdHRzAAAAAAAAABkAAAABAAAEAAAAAAEAAAoAAAAA
AQAABAAAAAABAAAAAAAAAAEAAAIAAAAAAQAACgAAAAABAAAEAAAAAAEAAAAAAAAAAQAAAgAAAAABAAAKAAAAAAEAAAQAAAAAAQAA
AAAAAAABAAACAAAAAAEAAAoAAAAAAQAABAAAAAABAAAAAAAAAAEAAAIAAAAAAQAACgAAAAABAAAEAAAAAAEAAAAAAAAAAQAAAgAA
AAABAAAKAAAAAAEAAAQAAAAAAQAAAAAAAAABAAACAAAAABxzdHNjAAAAAAAAAAEAAAABAAAAGQAAAAEAAAB4c3RzegAAAAAAAAAA
AAAAGQAAAt0AAAAOAAAADAAAAAwAAAAMAAAAFAAAAA4AAAAMAAAADAAAABQAAAAOAAAADAAAAAwAAAAUAAAADgAAAAwAAAAMAAAA
FAAAAA4AAAAMAAAADAAAABQAAAAOAAAADAAAAAwAAAAUc3RjbwAAAAAAAAABAAAAMAAAAGF1ZHRhAAAAWW1ldGEAAAAAAAAAIWhk
bHIAAAAAAAAAAG1kaXJhcHBsAAAAAAAAAAAAAAAALGlsc3QAAAAkqXRvbwAAABxkYXRhAAAAAQAAAABMYXZmNjMuMS4xMDI=`

// TestSproxyCloudOnlyAutoCleanRealE2E 验证 cloud_only=true 时 sproxy 转存后
// auto-clean task，download-manager 能从 transfer target 恢复成功。
func TestSproxyCloudOnlyAutoCleanRealE2E(t *testing.T) {
	base := strings.TrimRight(os.Getenv("SPROXY_E2E_URL"), "/")
	ak, sk, skid := os.Getenv("SPROXY_E2E_AK"), os.Getenv("SPROXY_E2E_SK"), os.Getenv("SPROXY_E2E_SKID")
	if base == "" || ak == "" || sk == "" || skid == "" {
		t.Skip("需要 SPROXY_E2E_URL/SPROXY_E2E_AK/SPROXY_E2E_SK/SPROXY_E2E_SKID")
	}
	mp4, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(tinyMP4Base64, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(mp4)
	}))
	defer fileSrv.Close()

	target := fmt.Sprintf("e2e/cloud-only-%d/video.mp4", time.Now().UnixNano())
	d := downloader.NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    base + "/api/cloud/download",
		AccessKey: ak, AccessKeySecret: sk, AccessKeyID: skid,
		TransferVolume: "main", TransferPath: target, CloudOnly: true,
		PollEvery: 200 * time.Millisecond, Timeout: 30 * time.Second, InsecureTLS: true,
	})
	obj := &model.DownloadObject{URL: fileSrv.URL + "/video.mp4", Extra: map[string]any{}}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	obj.RLock()
	transferURL, _ := obj.Extra["transfer_url"].(string)
	obj.RUnlock()
	if transferURL == "" {
		t.Fatal("transfer_url missing after auto-clean recovery")
	}
	fc := sproxyclient.NewFileClient(base,
		sproxyclient.WithVolume("main"), sproxyclient.WithAccessKey(ak, sk),
		sproxyclient.WithAccessKeyID(skid), sproxyclient.WithInsecureTLS(),
		sproxyclient.WithTimeout(30*time.Second),
	)
	t.Cleanup(func() { _ = fc.Delete(context.Background(), target, "") })
	if _, err := fc.Stat(context.Background(), target); err != nil {
		t.Fatalf("transfer target missing: %v", err)
	}
}
