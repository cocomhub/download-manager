// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import _ "embed" // embed player_util.js at compile time for PlayerUtilJS

// PlayerUtilJS is the embedded JavaScript utility for the tktube player.
//
//go:embed player_util.js
var PlayerUtilJS string
