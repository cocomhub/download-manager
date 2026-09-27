// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "strings"

// ProxyKind 标识代理的接入类型。代理 URL 字符串可带前缀标注类型：
//
//   - 无前缀：ProxyKindStandard（默认）——标准 HTTP 代理语义（Go http.Transport.Proxy）：
//     http 目标发绝对 URI（GET http://host/path），https 目标走 CONNECT 隧道（TLS 端到端）。
//     与 curl/wget -e use_proxy 一致，兼容 sproxy http-proxy / 常见代理服务器。
//   - "gateway:" 前缀：ProxyKindGateway——网关式转发（旧行为）：把目标域名拼进代理 URL 路径
//     （http://proxy/<host>/<path>），适用于以网关方式暴露下载通道的旧式代理。
type ProxyKind int

const (
	// ProxyKindStandard 标准 HTTP 代理（默认，无前缀）。
	ProxyKindStandard ProxyKind = iota
	// ProxyKindGateway 网关式转发（"gateway:" 前缀，旧行为）。
	ProxyKindGateway
)

// gatewayPrefix 是网关式代理 URL 的前缀标识。
const gatewayPrefix = "gateway:"

// String 返回代理类型的可读名称（配置与日志用）。
func (k ProxyKind) String() string {
	switch k {
	case ProxyKindGateway:
		return "gateway"
	default:
		return "standard"
	}
}

// ParseProxyKind 解析代理 URL 字符串，返回其类型与剥离前缀后的真实 URL。
// 规则：
//   - 空串 → (ProxyKindStandard, "")
//   - "gateway:http://..." → (ProxyKindGateway, "http://...")
//   - 其余（无前缀或未知前缀）→ (ProxyKindStandard, 原样)，不误伤 URL 本身。
func ParseProxyKind(proxyURL string) (ProxyKind, string) {
	if proxyURL == "" {
		return ProxyKindStandard, ""
	}
	if rest, ok := strings.CutPrefix(proxyURL, gatewayPrefix); ok {
		return ProxyKindGateway, rest
	}
	return ProxyKindStandard, proxyURL
}

// KindOfProxy 返回代理 URL 的类型（仅判断，不剥离）。
func KindOfProxy(proxyURL string) ProxyKind {
	k, _ := ParseProxyKind(proxyURL)
	return k
}
