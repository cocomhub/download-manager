// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"

	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

// SproxyCloudDownloader 经 sproxy cloud download API 提交 URL 到云端下载/转存。
//
// 定位（用户要求）：download-manager 只做「原始任务解析和装配」——不实现云端下载策略
// （分享签名/转存/账号池/分片）——而是把 URL 交给 sproxy 的 POST /api/cloud/download
// （sproxy 侧 downloaderFor 按 URL 自动发现后端）。**通用**：任意 URL 均可提交，PikPak
// 分享链接（keepshare/mypikpak）为特化优先来源（sproxy 侧走 hybrid 分片并行）。
// dm 侧只负责：选择 URL → 提交任务 → 轮询完成 → 转存/下载到本地（cloud_only 控制）。
//
// 认证（item6 SproxySig 接入）：优先 SproxySig 签名认证（AccessKey/SK/skey-id，
// 复用 sproxy pkg/client.FileClient——同 sclient 同一库，自带 v2 签名 + SK 轮换）；
// 未配 SproxySig 三件套时回落旧 Bearer 直连（APIToken，向后兼容零回归）。
type SproxyCloudDownloader struct {
	apiURL       string        // sproxy 云下载 API（如 http://127.0.0.1:8080/api/cloud/download）
	apiToken     string        // sproxy API 认证 token（可空；SproxySig 配置后忽略）
	pollEvery    time.Duration // 任务轮询间隔
	timeout      time.Duration // 总超时
	httpClient   *http.Client
	sig          *sproxyclient.FileClient // SproxySig 凭据客户端（nil = Bearer 路径）；自带签名+轮换
	sigEnabled   bool
	ak           string // SproxySig AccessKey（启动验证 ListAccessKeys 用）
	skid         string // SproxySig SK 条目 ID（启动验证取过期时间用）
	transferVol  string // 转存目标卷（非空 → 提交带 transfer；默认留 cloud 桶）
	transferPath string // 转存目标路径（卷内相对路径，可含子目录，如 xxx/xxxx.mp4）
	cloudOnly    bool   // true=仅云端不下载本地；默认 false=下载到本地 SavePath
	// 主动 SK 轮换调度（提前 24h 每小时直到成功）：
	now         func() time.Time // 时钟注入（测试可控）
	rotateMu    sync.Mutex
	expireAt    time.Time // 当前 SK 过期时间（RenewAccessKey/启动验证 记录）
	lastRotate  time.Time // 上次轮换时刻（每小时限频）
	verified    bool      // 启动验证通过（签名链路有效才启用 SproxySig 路径；否则 degraded）
	apiURLSet   bool      // api_url 是否显式配置（未配则拒绝投递，避免发往缺省地址）
	partialCred bool      // SproxySig 三件套是否部分配置（非全有/全空→配置错误）
	// 启动验证懒重验限频（sproxy 晚于 dm 启动 / 瞬时不可用后自愈）。
	verifyMu      sync.Mutex
	lastVerifyTry time.Time
	// 下载上下文（manager 经 ContextInjecter 注入；用于取消/停止传播）。
	ctxMu sync.Mutex
	dlCtx context.Context
	// urlCtx 按 URL 注入的下载上下文（core.ContextInjecterFor）：共享单实例并发
	// 下载时避免单字段 dlCtx 被兄弟下载覆盖导致的误取消。
	urlCtx map[string]context.Context
	// per-URL 取消注册表（manager 单对象取消/删除经 Cancel 触发）。
	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

// Extra 键名常量（避免同文件重复字面量）。
const extraKeyCloudTaskID = "cloud_task_id"

// NewSproxyCloudDownloader 创建 sproxy 云端下载器。
// 配置了 access_key/access_key_secret → 用 FileClient（SproxySig 签名认证）；否则旧 Bearer。
func NewSproxyCloudDownloader(cfg config.SproxyCloudConfig) *SproxyCloudDownloader {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080/api/cloud/download"
	}
	pollEvery := cfg.PollEvery
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Hour
	}
	headerOK := strings.TrimRight(apiURL, "/")
	// 评审 P0-1：SproxySig 需三件套齐（AK/SK/ID）才启用——缺 access_key_id 时 v2 签名
	// 必被服务端 401，半启用会造成「看似配好、实则全任务失败」静默失效。
	sigEnabled := cfg.AccessKey != "" && cfg.AccessKeySecret != "" && cfg.AccessKeyID != ""
	var sig *sproxyclient.FileClient
	if sigEnabled {
		baseURL := strings.TrimSuffix(headerOK, "/api/cloud/download")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8080"
		}
		opts := []sproxyclient.Option{
			sproxyclient.WithAccessKey(cfg.AccessKey, cfg.AccessKeySecret),
			sproxyclient.WithAccessKeyID(cfg.AccessKeyID),
		}
		// 不默认压缩 HTTP 超时：sproxy FileClient 默认 300s；pullback 的分块
		// （4~64MiB）在限速下需远超 30s，压到 30s 会使多 GB 拉回必失败
		// （对抗性评审 P1-3）。仅当显式配置 client_timeout 时覆盖。
		if cfg.ClientTimeout > 0 {
			opts = append(opts, sproxyclient.WithTimeout(cfg.ClientTimeout))
		}
		sig = sproxyclient.NewFileClient(baseURL, opts...)
	}
	d := &SproxyCloudDownloader{
		apiURL:       strings.TrimRight(apiURL, "/"),
		apiToken:     cfg.APIToken,
		pollEvery:    pollEvery,
		timeout:      timeout,
		httpClient:   &http.Client{Timeout: clientTimeout(cfg.ClientTimeout)},
		sig:          sig,
		sigEnabled:   sigEnabled,
		ak:           cfg.AccessKey,
		skid:         cfg.AccessKeyID,
		transferVol:  cfg.TransferVolume,
		transferPath: cfg.TransferPath,
		cloudOnly:    cfg.CloudOnly,
		now:          time.Now,
		apiURLSet:    cfg.APIURL != "",
		partialCred:  (cfg.AccessKey != "" || cfg.AccessKeySecret != "" || cfg.AccessKeyID != "") && !sigEnabled,
	}
	// 用户要求：启动时立刻确认签名链路有效才启用——同步带外验证（ListAccessKeys 200），
	// 失败仅 Warn + 标记 degraded（Download 显式报错，不静默降级）；成功则预热 expireAt
	// 使「过期前 24h 每小时轮换」调度立即生效。
	if sigEnabled {
		d.verifyOnStart()
	}
	return d
}

// tryReverify 限频（1min）重试启动验证；返回当前 verified 状态。
// 用于 sproxy 晚于 dm 启动或瞬时不可用后的自愈（对抗性评审 P2-5）。
func (d *SproxyCloudDownloader) tryReverify() bool {
	d.verifyMu.Lock()
	if time.Since(d.lastVerifyTry) < time.Minute {
		v := d.verified
		d.verifyMu.Unlock()
		return v
	}
	d.lastVerifyTry = time.Now()
	d.verifyMu.Unlock()
	d.verifyOnStart()
	d.verifyMu.Lock()
	defer d.verifyMu.Unlock()
	return d.verified
}

// verifyTimeout 是启动验证（verifyOnStart）的超时：独立且较短，避免 sproxy
// 不可达时阻塞 manager 构造/配置热更新（对抗性评审 P2-6）。
const verifyTimeout = 5 * time.Second

// verifyOnStart 启动时验证 SproxySig 签名链路有效：ListAccessKeys 返回 200 即凭据
// 有效（服务端真实鉴权）；成功时从 SK 列表取当前条目过期时间预热 expireAt（供提前
// 24h 每小时轮换调度）。失败 → verified=false（Download 显式报错，不静默降级）。
func (d *SproxyCloudDownloader) verifyOnStart() {
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	infos, err := d.sig.ListAccessKeys(ctx, d.ak)
	if err != nil {
		slog.Error("sproxy sig verify-on-start failed: 签名链路不可用，SproxySig 路径停用（任务将显式失败）",
			"err", err, "ak", d.ak)
		d.verifyMu.Lock()
		d.verified = false
		d.verifyMu.Unlock()
		return
	}
	d.verifyMu.Lock()
	d.verified = true
	d.verifyMu.Unlock()
	// 预热 expireAt：找当前 skeyID 条目过期；找不到取最晚过期（服务端多 SK 共存）。
	// expireAt 由 rotateMu 保护（与 rotateOnce/ensureRotatedBeforeSubmit 对称）。
	var expireAt, latest time.Time
	for _, info := range infos {
		if info.SKID == d.skid {
			expireAt = info.Expires
			break
		}
		if info.Expires.After(latest) {
			latest = info.Expires
		}
	}
	if expireAt.IsZero() {
		expireAt = latest
	}
	d.rotateMu.Lock()
	d.expireAt = expireAt
	d.rotateMu.Unlock()
	// 注意：不设 lastRotate——verifyOnStart 并未真正轮换 SK；若设了，会使紧接着
	// 的 401 应急轮换被“1h 限频”误判为已轮换而跳过（真实缺陷）。
	slog.Info("sproxy sig verified on start", "ak", d.ak, "expire_at", expireAt.Format(time.RFC3339))
}

// 主动 SK 轮换调度（用户确认策略：拿到凭证记录过期时间，提前 24h 开始每小时轮换直到成功）。
const (
	rotateAhead       = 24 * time.Hour // 到期前 24h 进入轮换窗口
	rotateIntervalGap = time.Hour      // 相邻两次轮换的最小间隔（每小时限频）
)

// clientTimeout 返回单请求超时：0 或负用默认 30s（可由 ClientTimeout 放宽）。
func clientTimeout(ct time.Duration) time.Duration {
	if ct <= 0 {
		return 30 * time.Second
	}
	return ct
}

// Name 返回下载器名称。
func (d *SproxyCloudDownloader) Name() string { return "sproxy_cloud" }

// statusURL 构造任务详情 URL（<base>/api/cloud/tasks/<id>）。
// 用 url.Parse 取 scheme+host 后拼固定路径，避免 api_url 的主机名/路径含 “download”
// 时 strings.Replace 误切 URL（对抗性评审 P2-1）。
func (d *SproxyCloudDownloader) statusURL(taskID string) string {
	if u, err := url.Parse(d.apiURL); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/api/cloud/tasks/" + url.PathEscape(taskID)
	}
	base := strings.TrimSuffix(strings.TrimRight(d.apiURL, "/"), "/api/cloud/download")
	return base + "/api/cloud/tasks/" + url.PathEscape(taskID)
}

// validateSaveName 校验提交给服务端的文件名（sproxy cloudfilename 仅接受单文件名：
// 含路径分隔符会被 400 拒绝；超长/纯点号也不合法）。空名 = 由服务端按 URL 推导。
func validateSaveName(name string) error {
	if name == "" {
		return nil
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("filename 不能含路径分隔符: %q", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("filename 非法: %q", name)
	}
	if len(name) > 200 {
		return fmt.Errorf("filename 过长（%d 字节）", len(name))
	}
	return nil
}

// SetMetadataFlusher 实现 manager 的可选接口：本下载器不产 ETag/checksum 元数据，
// 空实现以避免 manager 对每个对象打“Metadata flush not supported”误导 Warn
// （对抗性评审 P2-6）。
func (d *SproxyCloudDownloader) SetMetadataFlusher(_ func()) {}

// CloseIdleConnections 关闭底层 HTTP 客户端的空闲连接（停机 / 配置热更新时调用）。
func (d *SproxyCloudDownloader) CloseIdleConnections() {
	if d.httpClient != nil {
		d.httpClient.CloseIdleConnections()
	}
}

// Ensure SproxyCloudDownloader implements core.Downloader
var _ core.Downloader = &SproxyCloudDownloader{}

// Ensure SproxyCloudDownloader implements core.ContextInjecter（取消/停止传播）。
var _ core.ContextInjecter = &SproxyCloudDownloader{}

// SetContext 注入下载上下文（manager 每任务调用）——把取消/优雅停止传播到
// submit/poll/pullback（否则全程 context.Background() 会让取消与退出失效）。
// 注：downloader 为单例，与既有 adapter 同模式；并发多任务时先后 SetContext 会
// 互相覆盖（接口限制），此处仅保证单任务/进程停止场景的取消语义。
func (d *SproxyCloudDownloader) SetContext(ctx context.Context) {
	d.ctxMu.Lock()
	d.dlCtx = ctx
	d.ctxMu.Unlock()
}

// reqCtx 返回当前注入的下载上下文（未注入时 Background）。
func (d *SproxyCloudDownloader) reqCtx() context.Context {
	d.ctxMu.Lock()
	defer d.ctxMu.Unlock()
	if d.dlCtx != nil {
		return d.dlCtx
	}
	return context.Background()
}

// SetContextFor 按 URL 注入下载上下文（实现 core.ContextInjecterFor）：
// 共享单实例并发下载时，避免单字段 SetContext 被兄弟下载覆盖导致的误取消。
func (d *SproxyCloudDownloader) SetContextFor(url string, ctx context.Context) {
	if url == "" {
		return
	}
	d.ctxMu.Lock()
	if d.urlCtx == nil {
		d.urlCtx = make(map[string]context.Context)
	}
	d.urlCtx[url] = ctx
	d.ctxMu.Unlock()
}

// reqCtxFor 返回该 URL 的下载上下文：优先按 URL 注入，否则回落进程级注入。
func (d *SproxyCloudDownloader) reqCtxFor(url string) context.Context {
	d.ctxMu.Lock()
	defer d.ctxMu.Unlock()
	if c, ok := d.urlCtx[url]; ok && c != nil {
		return c
	}
	if d.dlCtx != nil {
		return d.dlCtx
	}
	return context.Background()
}

// clearContextFor 清理该 URL 的注入上下文（Download 结束时）。
func (d *SproxyCloudDownloader) clearContextFor(url string) {
	d.ctxMu.Lock()
	delete(d.urlCtx, url)
	d.ctxMu.Unlock()
}

// Cancel 取消指定 URL 的进行中下载（manager 单对象取消/删除时经类型断言调用）。
// 幂等：无在途任务时静默返回 nil。
func (d *SproxyCloudDownloader) Cancel(url string) error {
	d.cancelMu.Lock()
	cf := d.cancels[url]
	d.cancelMu.Unlock()
	if cf != nil {
		cf()
	}
	return nil
}

// registerCancel 登记某 URL 的取消函数（Download 开始时）。
func (d *SproxyCloudDownloader) registerCancel(url string, cf context.CancelFunc) {
	d.cancelMu.Lock()
	if d.cancels == nil {
		d.cancels = make(map[string]context.CancelFunc)
	}
	d.cancels[url] = cf
	d.cancelMu.Unlock()
}

// unregisterCancel 注销某 URL 的取消函数（Download 结束时）。
func (d *SproxyCloudDownloader) unregisterCancel(url string) {
	d.cancelMu.Lock()
	delete(d.cancels, url)
	d.cancelMu.Unlock()
}

// Download 把 obj 的 URL 提交到 sproxy cloud download，轮询完成后转存/下载产物。
//
// headers 透传给 submit（sproxy 若需 Referer/UA 等下载头，不丢失）。
//
// URL 来源（见 pickURL，按优先级）：
//  1. obj.Extra.magnet_list 里的 keepshare 分享链接（与 gopeed collectPikPakCandidates 对齐）
//  2. obj.Extra.files 里的 keepshare/mypikpak 分享链接
//  3. obj.URL 本身（**通用回落**：任意 URL 均可交给 sproxy，由其按 URL 自动发现后端）
func (d *SproxyCloudDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 配置校验（fail-closed，对抗性评审 P2-2/P2-3）：避免把分享 URL 投递给缺省地址
	// 或半配置凭据静默全失败。
	if !d.apiURLSet {
		return fmt.Errorf("%w: sproxy_cloud api_url 未配置（拒绝投递到缺省地址）", download.ErrNoTry)
	}
	if d.partialCred {
		return fmt.Errorf("%w: sproxy_cloud SproxySig 三件套需同时配置（access_key/access_key_secret/access_key_id 部分缺失）", download.ErrNoTry)
	}
	// 启动验证失败（且懒重验仍失败）→ 显式拒绝任务，不静默降级；
	// 但允许限频重验（sproxy 晚于 dm 启动/瞬时抖动可自愈）。
	if d.sigEnabled && !d.verified && !d.tryReverify() {
		return fmt.Errorf("%w: sproxy_cloud SproxySig 启动验证失败（签名链路不可用）；请检查 access_key/access_key_secret/access_key_id 配置", download.ErrNoTry)
	}
	shareURL := d.pickURL(obj)
	if shareURL == "" {
		return fmt.Errorf("%w: sproxy_cloud 无可提交的 URL（obj.URL 为空）", download.ErrNoTry)
	}

	// 1. 提交前：SproxySig 凭证到期前 24h 窗口内先主动轮换（每小时限频，失败不阻塞提交）
	if d.sigEnabled {
		_ = d.ensureRotatedBeforeSubmit()
	}
	// 派生 per-URL 下载上下文：使 manager 的单对象取消（Cancel(url)）与
	// 停机（SetContext/SetContextFor 注入的 ctx）都能中断 submit/poll/pullback。
	ctx, cancel := context.WithCancel(d.reqCtxFor(obj.URL))
	defer cancel()
	d.registerCancel(obj.URL, cancel)
	defer d.unregisterCancel(obj.URL)
	defer d.clearContextFor(obj.URL)

	// 1. 提交任务；若上次已提交（Extra 有 cloud_task_id）则复用，避免重试（如 pullback
	// 失败）时重复 submit 造成重复下载/转存（对抗性评审 P2-9：服务端仅对在途任务去重）。
	taskID := extraString(obj, extraKeyCloudTaskID)
	if taskID != "" {
		slog.Info("Sproxy cloud reuse existing task", "task_id", taskID, logutil.LogKeyURL, shareURL)
	} else {
		tid, serr := d.submit(ctx, shareURL, obj.SavePath, headers)
		if serr != nil {
			return fmt.Errorf("sproxy_cloud submit %s: %w", shareURL, serr)
		}
		taskID = tid
		slog.Info("Sproxy cloud task submitted", "task_id", taskID, logutil.LogKeyURL, shareURL)
		setExtra(obj, extraKeyCloudTaskID, taskID) // 提交即落坐标，供重试复用
	}

	// 2. 轮询直到完成（SproxySig 路径返回 CloudTask 供转存 URL 记录/拉回）
	task, err := d.pollResult(ctx, taskID)
	if err != nil {
		return fmt.Errorf("sproxy_cloud task %s: %w", taskID, err)
	}
	// 完成：置进度 100（否则 SSE/UI 恒 0%，对抗性评审 P2-7）。
	obj.SetProgress(100)
	// 3. 记录产物取用坐标（转存 URL + cloud 桶任务坐标）——无论走哪条路径，
	// 都让上层/人工有据可取（对抗性评审 P1-2：默认无产物时也要有线索）。
	if task != nil {
		if task.TransferURL != "" {
			setExtra(obj, "transfer_url", task.TransferURL)
		}
		if task.ID != "" {
			setExtra(obj, extraKeyCloudTaskID, task.ID)
		}
		if task.Filename != "" {
			setExtra(obj, "cloud_task_filename", task.Filename)
		}
	}
	// 4. 本地下载（默认）：cloud_only=false 时用 kind=cloud_task 把原始文件拉到 SavePath。
	// 无法执行时显式报错（对抗性评审 P1-1：静默跳过会让对象标 completed 但本地无产物）。
	if !d.cloudOnly {
		if d.sig == nil {
			return fmt.Errorf("sproxy_cloud: 本地下载需要 SproxySig 凭据（access_key/access_key_secret/access_key_id）；或设 cloud_only: true 只留云端")
		}
		if obj.SavePath == "" {
			return fmt.Errorf("sproxy_cloud: 本地下载需要对象 SavePath（为空无法落盘）；或设 cloud_only: true 只留云端")
		}
		if task == nil || task.Filename == "" {
			return fmt.Errorf("sproxy_cloud: pullback 缺少任务产物坐标（task/filename 为空）")
		}
		if perr := d.pullBackToSavePath(ctx, task, obj.SavePath); perr != nil {
			return fmt.Errorf("sproxy_cloud pullback %s: %w", task.ID, perr)
		}
	}
	return nil
}

// pullBackToSavePath 用 FileClient 以 kind=cloud_task 下载任务原始文件到本地 SavePath。
func (d *SproxyCloudDownloader) pullBackToSavePath(ctx context.Context, task *sproxyclient.CloudTask, savePath string) error {
	// 原子落盘：先写 .partial，成功后 rename 到 savePath。直接写 savePath 时，进程被强杀/
	// 中断会留下“尺寸像完成、内容却是零/混合”的假完成文件（对抗性评审 P2-5）。
	tmp := savePath + ".partial"
	// 服务端 cloud_task 下载 filename=<taskID>/<file>（resolveCloudTaskPath 契约）
	if err := d.sig.ChunkedDownload(ctx, task.ID+"/"+task.Filename, tmp,
		sproxyclient.WithChunkedKind(sproxyclient.DownloadKindCloudTask)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, savePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("pullback rename %s: %w", savePath, err)
	}
	return nil
}

// pickURL 从 obj 选择提交给 sproxy 的 URL（PikPak 分享优先，否则 obj.URL）：
// pickURL 选择提交给 sproxy 云端下载的 URL：
//  1. PikPak 分享链接优先（magnet_list[].keepshare → files[].url → obj.URL 若为分享链接）
//  2. 否则回落到 obj.URL——**通用**：任意 URL 都可交给 sproxy cloud download，
//     由其 downloaderFor 按 URL 自动发现后端（非只做 PikPak）。
//
// 注：magnet_list 里的纯 magnet（`magnet:`/`bt:`）走 Gopeed P2P 分支，云端下载解析不回
// 分享直链 → 不采集（正确忽略），此时回落 obj.URL。
func (d *SproxyCloudDownloader) pickURL(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	// ① magnet_list[].keepshare（与 gopeed 同源解析）
	if u := d.shareFromMagnetList(obj); u != "" {
		return u
	}
	// ② files[] 分享链接
	if u := d.shareFromFiles(obj); u != "" {
		return u
	}
	// ③ 通用回落：obj.URL（任意 URL 交 sproxy 云端下载）
	return strings.TrimSpace(obj.URL)
}

// shareFromMagnetList 从 magnet_list 取第一个 keepshare 分享直链（keepshare.org/<id>/magnet: 形态）。
func (d *SproxyCloudDownloader) shareFromMagnetList(obj *model.DownloadObject) string {
	raw, ok := obj.Extra["magnet_list"].([]map[string]string)
	if ok {
		for _, m := range raw {
			if u := strings.TrimSpace(m["keepshare"]); u != "" && isShareURL(u) {
				return u
			}
		}
		return ""
	}
	if fa, ok := obj.Extra["magnet_list"].([]any); ok {
		for _, it := range fa {
			if m, ok2 := it.(map[string]any); ok2 {
				if u, _ := m["keepshare"].(string); u != "" && isShareURL(u) {
					return u
				}
			}
		}
	}
	return ""
}

// shareFromFiles 从 obj.Extra.files 取第一个分享链接（keepshare/mypikpak）。
func (d *SproxyCloudDownloader) shareFromFiles(obj *model.DownloadObject) string {
	if raw, ok := obj.Extra["files"].([]map[string]string); ok {
		for _, f := range raw {
			if u := strings.TrimSpace(f["url"]); u != "" && isShareURL(u) {
				return u
			}
		}
	}
	if fa, ok := obj.Extra["files"].([]any); ok {
		for _, it := range fa {
			if fm, ok := it.(map[string]any); ok {
				if u, _ := fm["url"].(string); u != "" && isShareURL(u) {
					return u
				}
			}
		}
	}
	return ""
}

// isShareURL 判断是否为 PikPak 分享 URL（keepshare / mypikpak / mypikpak.net）。
func isShareURL(u string) bool {
	low := strings.ToLower(u)
	return strings.Contains(low, "keepshare.org/") ||
		strings.Contains(low, "keepshare.cc/") ||
		strings.Contains(low, "mypikpak.com/s/") ||
		strings.Contains(low, "mypikpak.net/s/")
}

// submit POST 到 sproxy cloud download，返回 task id。
// headers 透传给请求（sproxy 若需 Referer/UA 等下载头不丢失）。
// SproxySig 路径用 FileClient（RequestRaw 透传 headers + 签名）；Bearer 路径走 HTTP 直连。
func (d *SproxyCloudDownloader) submit(ctx context.Context, shareURL, savePath string, headers map[string]string) (string, error) {
	if d.sigEnabled && d.sig != nil {
		return d.submitSig(ctx, shareURL, savePath, headers)
	}
	body := map[string]any{"url": shareURL}
	if savePath != "" {
		name := baseName(savePath)
		if err := validateSaveName(name); err != nil {
			return "", err
		}
		body["filename"] = name
	}
	if tr := d.transferSpec(); tr != nil {
		body["transfer"] = tr
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.apiURL, strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if k == "" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("submit response missing task id")
	}
	return out.ID, nil
}

// submitSig 用 SproxySig 客户端提交（RequestRaw 带签名 + 透传自定义 headers）。
// TransferVolume 非空 → 请求体带 transfer（master 新三行为：转存到卷）。
// 401（签名失效）触发轮换后**有限次重试**（评审 P1-1：无界递归 → DoS）；
// 重试次数耗尽后返回显式错误（不静默降级）。
func (d *SproxyCloudDownloader) submitSig(ctx context.Context, shareURL, savePath string, headers map[string]string) (string, error) {
	const maxSubmitRetry = 2
	for attempt := 0; attempt <= maxSubmitRetry; attempt++ {
		body := map[string]any{"url": shareURL}
		if savePath != "" {
			name := baseName(savePath)
			if err := validateSaveName(name); err != nil {
				return "", err
			}
			body["filename"] = name
		}
		if tr := d.transferSpec(); tr != nil {
			body["transfer"] = tr
		}
		b, _ := json.Marshal(body)
		hdr := make(http.Header)
		for k, v := range headers {
			if k == "" || v == "" {
				continue
			}
			hdr.Set(k, v)
		}
		resp, err := d.sig.RequestRaw(ctx, http.MethodPost, "/api/cloud/download", strings.NewReader(string(b)), hdr)
		if err != nil {
			return "", err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// 仅 401 触发轮换重试（有限次）；其它错误直接返回
			if resp.StatusCode == http.StatusUnauthorized && attempt < maxSubmitRetry {
				if rerr := d.rotateOnce(); rerr == nil {
					continue // 换新 SK 后重试
				}
			}
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return "", fmt.Errorf("decode submit response: %w", err)
		}
		if out.ID == "" {
			return "", fmt.Errorf("submit response missing task id")
		}
		return out.ID, nil
	}
	return "", fmt.Errorf("submit failed after %d attempts", maxSubmitRetry+1)
}

// sleepCtx 等待 d 时长或 ctx 结束；返回 false 表示 ctx 已取消（调用方应停止轮询）。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// extraString 读 obj.Extra 字符串字段（加锁）。
func extraString(obj *model.DownloadObject, key string) string {
	obj.RLock()
	defer obj.RUnlock()
	s, _ := obj.Extra[key].(string)
	return s
}

// setExtra 写 obj.Extra 字段（加锁，惰性初始化 map）。
func setExtra(obj *model.DownloadObject, key string, value any) {
	obj.Lock()
	if obj.Extra == nil {
		obj.Extra = make(map[string]any)
	}
	obj.Extra[key] = value
	obj.Unlock()
}

// transferSpec 构造转存请求体（volume 必填；path 可选，支持卷内子目录/重命名，
// 如 xxx/xxxx.mp4）。返回 nil = 不转存。
func (d *SproxyCloudDownloader) transferSpec() map[string]any {
	if d.transferVol == "" {
		return nil
	}
	tr := map[string]any{"volume": d.transferVol}
	if d.transferPath != "" {
		tr["path"] = d.transferPath
	}
	return tr
}

// rotateOnce 调用一次 RenewAccessKey（热替换新 SK；失败 Warn 下轮重试）。
// 成功后记录新 SK 的过期时间（供提前 24h 主动轮换调度）。
//
// single-flight：持锁跨整个 renew——并发调用（多任务同日进入 24h 窗口/同时 401）
// 中先到者真正 renew，其余阻塞到其完成；完成后因 lastRotate 已刷新（<1h）而直接
// 返回成功（此时会话已是新 SK，语义正确），既无并发重复 renew（对抗性评审 P1-4），
// 也不会让等待者拿旧 SK 白消耗重试次数（对抗性评审 P2-4）。
func (d *SproxyCloudDownloader) rotateOnce() error {
	if d.sig == nil {
		return fmt.Errorf("sproxy sig client not initialized")
	}
	d.rotateMu.Lock()
	defer d.rotateMu.Unlock()
	now := d.now()
	// 近期已轮换（1h 内）→ 复用结果，不重复 renew（并发等待者在此路径返回）。
	if !d.lastRotate.IsZero() && now.Sub(d.lastRotate) < rotateIntervalGap {
		return nil
	}
	res, err := d.sig.RenewAccessKey(d.reqCtx())
	if err != nil {
		slog.Warn("sproxy sig renew failed", "err", err)
		return err
	}
	slog.Info("sproxy sig key rotated", "sk_id", res.SKID, "expires", res.ExpiresAt)
	if !res.ExpiresAt.IsZero() {
		d.expireAt = res.ExpiresAt
	}
	d.lastRotate = now
	return nil
}

// ensureRotatedBeforeSubmit 提交前检查：若凭证进入「到期前 24h 窗口」且距上次轮换
// >1h，触发一次轮换（直到成功；失败仅 Warn，下个任务/下小时重试——不阻塞提交）。
// 用于推进「提前 24h 每小时轮换直到成功」的调度。
func (d *SproxyCloudDownloader) ensureRotatedBeforeSubmit() error {
	if d.sig == nil {
		return nil
	}
	d.rotateMu.Lock()
	expireAt := d.expireAt
	lastRotate := d.lastRotate
	d.rotateMu.Unlock()
	now := d.now()
	// 未记录到期时间（尚未 renew 过）→ 无可调度；等 401 触发
	if expireAt.IsZero() {
		return nil
	}
	// 距到期 >24h → 不轮换
	if now.Add(rotateAhead).Before(expireAt) {
		return nil
	}
	// 距上次轮换 <1h → 限频跳过
	if !lastRotate.IsZero() && now.Sub(lastRotate) < rotateIntervalGap {
		return nil
	}
	// 进窗口：每小时轮换直到成功（失败返错，由调用方决定是否继续）
	return d.rotateOnce()
}

// poll 轮询 sproxy cloud download 任务状态直到 done/failed/timeout。
// 非 2xx 且非 timeout 的错误（404 任务不存在/服务端异常）连续失败 N 次短路返回，
// 避免无限 sleep 到总超时（默认 3h）空等。
// SproxySig 路径：遇 401（签名失效）触发 RenewAccessKey 轮换后重试（pollWithRotate）。
func (d *SproxyCloudDownloader) poll(ctx context.Context, taskID string) error {
	_, err := d.pollResult(ctx, taskID)
	return err
}

// pollResult 轮询直到完成，返回最终 CloudTask（含 transfer_url / filename 供转存记录与拉回）。
// SproxySig 路径：遇 401（签名失效）触发 RenewAccessKey 轮换后重试（pollWithRotate）。
func (d *SproxyCloudDownloader) pollResult(ctx context.Context, taskID string) (*sproxyclient.CloudTask, error) {
	if d.sigEnabled && d.sig != nil {
		return d.pollWithRotateResult(ctx, taskID)
	}
	const maxConsecutiveErr = 3
	deadline := time.Now().Add(d.timeout)
	statusURL := d.statusURL(taskID)
	consecErr := 0
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s", d.timeout)
		}
		var out struct {
			Status      string `json:"status"`
			TransferURL string `json:"transfer_url"`
			Filename    string `json:"filename"`
		}
		if err := d.getJSON(ctx, statusURL, &out); err != nil {
			// 轮询失败（404 任务不存在/服务端异常）——连续失败 N 次短路返回，避免 3h 空等
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			if !sleepCtx(ctx, d.pollEvery) {
				return nil, ctx.Err()
			}
			continue
		}
		consecErr = 0
		switch out.Status {
		case "completed", "done":
			return &sproxyclient.CloudTask{ID: taskID, Status: out.Status, TransferURL: out.TransferURL, Filename: out.Filename}, nil
		case "failed", "error", "cancelled":
			return nil, fmt.Errorf("task status %q", out.Status)
		}
		if !sleepCtx(ctx, d.pollEvery) {
			return nil, ctx.Err()
		}
	}
}

// pollWithRotateResult 用 SproxySig 客户端轮询（GetCloudTask），**仅 401**（签名失效）
// 才触发 RenewAccessKey 轮换（评审 P1-A/P1-2：5xx/网络错误不轮换，防凭据风暴）；
// 轮换成功后用新 SK 继续轮询，连续错误仍计数短路。
func (d *SproxyCloudDownloader) pollWithRotateResult(ctx context.Context, taskID string) (*sproxyclient.CloudTask, error) {
	const maxConsecutiveErr = 3
	const maxPollRotate = 2 // 单次轮询会话的轮换次数上限（防持久 401 → renew 风暴）
	deadline := time.Now().Add(d.timeout)
	consecErr := 0
	rotateTries := 0
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s", d.timeout)
		}
		task, err := d.sig.GetCloudTask(ctx, taskID)
		if err != nil {
			// 404（任务不存在）→ 连续计数短路；其它（500/网络）同样计数——不轮换
			if !isUnauthorizedErr(err) {
				consecErr++
				if consecErr >= maxConsecutiveErr {
					return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
				}
				if !sleepCtx(ctx, d.pollEvery) {
					return nil, ctx.Err()
				}
				continue
			}
			// 仅 401：轮换后重试——单次轮询会话有轮换次数上限（对抗性评审
			// P1-3：无上限会在持久 401 下每 pollEvery 触发一次 renew 风暴）。
			if rotateTries < maxPollRotate {
				rotateTries++
				if rerr := d.rotateOnce(); rerr == nil {
					if !sleepCtx(ctx, d.pollEvery) {
						return nil, ctx.Err()
					}
					continue
				}
			}
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			if !sleepCtx(ctx, d.pollEvery) {
				return nil, ctx.Err()
			}
			continue
		}
		consecErr = 0
		switch task.Status {
		case "completed", "done":
			return task, nil
		case "failed", "error", "cancelled":
			return nil, fmt.Errorf("task status %q", task.Status)
		}
		if !sleepCtx(ctx, d.pollEvery) {
			return nil, ctx.Err()
		}
	}
}

// isUnauthorizedErr 判断错误是否为 401（签名失效）。
// go.mod pin 的 sproxy 版本已导出 ErrUnauthorized 哨兵（HTTP 401 → errors.Is 精确命中）；
// 保留 401 文案回退以兼容旧版 sproxy。
func isUnauthorizedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sproxyclient.ErrUnauthorized) {
		return true
	}
	// 回退：旧版 sproxy 无哨兵时按 401 文案判断。
	return strings.Contains(err.Error(), "(HTTP 401)")
}

// apiBase 返回 sproxy 服务基地址（apiURL 去掉 /api/cloud/download）。
// getJSON GET 请求并解析 JSON。
func (d *SproxyCloudDownloader) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// baseName 返回路径的文件名（不含目录）。
func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
