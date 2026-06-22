// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build no_mongo

package storage

// InitMongoClients is a no-op stub when built without mongo support.
func InitMongoClients(configs []struct{ Name, URI string }) error {
	return nil
}

// CloseAllMongoClients is a no-op stub when built without mongo support.
func CloseAllMongoClients() { //nolint:unused // intentional stub for no_mongo build tag
}
