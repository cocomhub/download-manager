// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cocomhub/download-manager/pkg/m3u8d"
)

func main() {
	// 瑙ｆ瀽鍛戒护琛屽弬鏁?	inputURL := flag.String("i", "", "杈撳叆m3u8 URL (蹇呴渶)")
	outputFile := flag.String("o", "output.mp4", "杈撳嚭鏂囦欢")
	userAgent := flag.String("user_agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36", "User-Agent澶?)
	cookie := flag.String("cookie", "", "鑷畾涔塁ookie锛屾牸寮? 'Cookie1=Value1; Cookie2=Value2'锛堥粯璁ょ┖锛岄渶杩愯鏃朵紶鍏ワ級")
	headers := flag.String("headers", "", "鑷畾涔夎姹傚ご锛屾牸寮? 'Header1: Value1; Header2: Value2'")
	concurrency := flag.Int("c", 4, "骞跺彂涓嬭浇鏁?)
	maxRetries := flag.Int("retry", 3, "澶辫触閲嶈瘯娆℃暟")
	timeout := flag.Int("timeout", 300, "瓒呮椂鏃堕棿(绉?")
	keepFiles := flag.Bool("keep", false, "淇濈暀涓嬭浇鐨勪复鏃舵枃浠?)
	workDir := flag.String("dir", "", "宸ヤ綔鐩綍(榛樿鑷姩鐢熸垚)")
	verbose := flag.Bool("v", true, "鏄剧ず璇︾粏杈撳嚭")
	ffmpegArgs := flag.String("args", "-c copy -bsf:a aac_adtstoasc -movflags +faststart -f mp4", "浼犻€掔粰ffmpeg鐨勫弬鏁?)

	flag.Parse()

	// 楠岃瘉蹇呴渶鍙傛暟
	if *inputURL == "" {
		fmt.Println("閿欒: 蹇呴』鎸囧畾杈撳叆URL (-i)")
		flag.Usage()
		os.Exit(1)
	}

	// 瑙ｆ瀽headers
	headerMap := make(map[string]string)
	if *headers != "" {
		pairs := strings.SplitSeq(*headers, ";")
		for pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(kv) == 2 {
				headerMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	if *cookie != "" {
		headerMap["Cookie"] = *cookie
	}

	// 瑙ｆ瀽ffmpeg鍙傛暟
	var ffmpegArgsList []string
	if *ffmpegArgs != "" {
		args := strings.Fields(*ffmpegArgs)
		ffmpegArgsList = args
	}

	// 鍒涘缓閰嶇疆
	config := &m3u8d.DownloadConfig{
		InputURL:    *inputURL,
		OutputFile:  *outputFile,
		UserAgent:   *userAgent,
		Headers:     headerMap,
		Concurrency: *concurrency,
		MaxRetries:  *maxRetries,
		WorkDir:     *workDir,
		KeepFiles:   *keepFiles,
		FFmpegArgs:  ffmpegArgsList,
		Timeout:     time.Duration(*timeout) * time.Second,
		Verbose:     *verbose,
	}

	// 鍒涘缓涓嬭浇鍣?	downloader, err := m3u8d.NewM3U8Downloader(config)
	if err != nil {
		fmt.Printf("鍒濆鍖栧け璐? %v\n", err)
		os.Exit(1)
	}

	// 鍒涘缓涓婁笅鏂?	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 鎹曡幏涓柇淇″彿
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
		fmt.Println("\n姝ｅ湪閫€鍑?..")
	}()

	// 涓嬭浇鎵€鏈夋枃浠?	fmt.Println("寮€濮嬩笅杞借祫婧?..")
	localM3U8, err := downloader.DownloadAll(ctx)
	if err != nil {
		fmt.Printf("涓嬭浇澶辫触: %v\n", err)
		os.Exit(1)
	}

	// 杞爜
	fmt.Println("涓嬭浇瀹屾垚锛屽紑濮嬭浆鐮?..")
	if err := downloader.ConvertToMP4(ctx, localM3U8); err != nil {
		fmt.Printf("杞爜澶辫触: %v\n", err)
		os.Exit(1)
	}

	// 娓呯悊
	if err := downloader.Cleanup(); err != nil {
		fmt.Printf("娓呯悊澶辫触: %v\n", err)
	}

	fmt.Printf("瀹屾垚! 杈撳嚭鏂囦欢: %s\n", config.OutputFile)
}
