// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	rootDir = flag.String("root", "./downloads", "root directory to check")
)

func main() {
	flag.Parse()

	recordedFiles := make(map[string][]string)

	err := filepath.WalkDir(*rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path %s: %v\n", path, err)
			return err
		}
		if d.IsDir() {
			return nil
		}
		fmt.Println(path)
		// Extract task info from filename
		taskName := extractTkTaskName(d.Name())
		if _, ok := recordedFiles[taskName]; ok {
			return nil
		}
		recordedFiles[taskName] = append(recordedFiles[taskName], d.Name())
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}
}

func extractTkTaskName(filename string) string {
	fields := strings.Fields(filename)
	if len(fields) > 0 {
		return fields[0]
	}
	return filename
}
