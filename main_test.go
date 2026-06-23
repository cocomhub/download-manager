// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestParseFlags_CLIRunModeOverridesOthers(t *testing.T) {
	res, err := parseFlags([]string{"--run-mode", "full", "--ui-only"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !res.RunModeSet || res.RunMode != config.RunModeFull {
		t.Fatalf("run mode = %v (set=%v), want full set", res.RunMode, res.RunModeSet)
	}
}

func TestParseFlags_UIOnlyWhenNoRunMode(t *testing.T) {
	res, err := parseFlags([]string{"--ui-only"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !res.RunModeSet || res.RunMode != config.RunModeUI {
		t.Fatalf("run mode = %v (set=%v), want ui set", res.RunMode, res.RunModeSet)
	}
}

func TestParseFlags_EnvRunModeUsedWhenNoCLI(t *testing.T) {
	t.Setenv("DM_RUN_MODE", "ui")
	res, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !res.RunModeSet || res.RunMode != config.RunModeUI {
		t.Fatalf("run mode = %v (set=%v), want ui set", res.RunMode, res.RunModeSet)
	}
}

func TestParseFlags_EnvUIOnlyUsedWhenNoCLIAndNoRunModeEnv(t *testing.T) {
	t.Setenv("DM_UI_ONLY", "true")
	res, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !res.RunModeSet || res.RunMode != config.RunModeUI {
		t.Fatalf("run mode = %v (set=%v), want ui set", res.RunMode, res.RunModeSet)
	}
}

func TestParseFlags_InvalidValueFallbackFull(t *testing.T) {
	res, err := parseFlags([]string{"--run-mode", "weird"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if !res.RunModeSet || res.RunMode != config.RunModeFull {
		t.Fatalf("run mode = %v (set=%v), want full set", res.RunMode, res.RunModeSet)
	}
}

func TestParseFlags_NoProvided_NotSet(t *testing.T) {
	res, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if res.RunModeSet {
		t.Fatalf("RunModeSet = true, want false when no flags/env")
	}
}
