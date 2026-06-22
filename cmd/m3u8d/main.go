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
	// 解析命令行参数
	inputURL := flag.String("i", "", "输入m3u8 URL (必需)")
	outputFile := flag.String("o", "output.mp4", "输出文件")
	userAgent := flag.String("user_agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36", "User-Agent头")
	cookie := flag.String("cookie", "", "自定义Cookie，格式: 'Cookie1=Value1; Cookie2=Value2'（默认空，需运行时传入）")
	headers := flag.String("headers", "", "自定义请求头，格式: 'Header1: Value1; Header2: Value2'")
	concurrency := flag.Int("c", 4, "并发下载数")
	maxRetries := flag.Int("retry", 3, "失败重试次数")
	timeout := flag.Int("timeout", 300, "超时时间(秒)")
	keepFiles := flag.Bool("keep", false, "保留下载的临时文件")
	workDir := flag.String("dir", "", "工作目录(默认自动生成)")
	verbose := flag.Bool("v", true, "显示详细输出")
	ffmpegArgs := flag.String("args", "-c copy -bsf:a aac_adtstoasc -movflags +faststart -f mp4", "传递给ffmpeg的参数")

	flag.Parse()

	// 验证必需参数
	if *inputURL == "" {
		fmt.Println("错误: 必须指定输入URL (-i)")
		flag.Usage()
		os.Exit(1)
	}

	// 解析headers
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

	// 解析ffmpeg参数
	var ffmpegArgsList []string
	if *ffmpegArgs != "" {
		args := strings.Fields(*ffmpegArgs)
		ffmpegArgsList = args
	}

	// 创建配置
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

	// 创建下载器
	downloader, err := m3u8d.NewM3U8Downloader(config)
	if err != nil {
		fmt.Printf("初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 捕获中断信号
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
		fmt.Println("\n正在退出...")
	}()

	// 下载所有文件
	fmt.Println("开始下载资源...")
	localM3U8, err := downloader.DownloadAll(ctx)
	if err != nil {
		fmt.Printf("下载失败: %v\n", err)
		os.Exit(1)
	}

	// 转码
	fmt.Println("下载完成，开始转码...")
	if err := downloader.ConvertToMP4(ctx, localM3U8); err != nil {
		fmt.Printf("转码失败: %v\n", err)
		os.Exit(1)
	}

	// 清理
	if err := downloader.Cleanup(); err != nil {
		fmt.Printf("清理失败: %v\n", err)
	}

	fmt.Printf("完成! 输出文件: %s\n", config.OutputFile)
}
