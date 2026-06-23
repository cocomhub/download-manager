// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "errors"

// ErrNoTry 琛ㄧず娌℃湁閲嶈瘯娆℃暟鍓╀綑锛屼笅杞藉簲缁堟銆?var ErrNoTry = errors.New("no try left")

// IsNoTry 鍒ゆ柇閿欒鏄惁涓?ErrNoTry 鎴栧叾鍖呰銆?func IsNoTry(err error) bool {
	return errors.Is(err, ErrNoTry)
}
