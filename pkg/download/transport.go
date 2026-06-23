// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

// Transport 鎺ュ彛灏佽搴曞眰 HTTP 浼犺緭灞傦紝鏀寔浠ｇ悊鍜岃嚜瀹氫箟璇锋眰/鍝嶅簲澶勭悊銆?type Transport interface {
	// Name 杩斿洖浼犺緭灞傜殑鍚嶇О銆?	Name() string

	// RoundTrip 鎵ц涓€娆″畬鏁寸殑 HTTP 寰€杩旓紝杩斿洖鍝嶅簲鎴栭敊璇€?	RoundTrip(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
}
