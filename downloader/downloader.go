package downloader

import (
	"crypto/md5"
	"download-manager/config"
	"download-manager/core"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
)

func New(config config.DownloaderConfig) core.Downloader {
	switch config.Type {
	case "wget":
		return NewWgetDownloader(config)
	default:
		return NewNativeHTTPDownloader(config)
	}
}

// ComputeFileMD5 计算文件的MD5校验值，返回Base64和十六进制两种格式
func ComputeFileMD5(filePath string) (string, string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	// 创建MD5哈希器
	hasher := md5.New()

	// 将文件内容拷贝到哈希器，适合大文件[1,3](@ref)
	if _, err := io.Copy(hasher, file); err != nil {
		return "", "", err
	}

	// 计算哈希值
	hashBytes := hasher.Sum(nil)

	// 转换为Base64编码（常见于HTTP头部）
	base64MD5 := base64.StdEncoding.EncodeToString(hashBytes)
	// 转换为十六进制字符串（便于阅读比较）
	hexMD5 := hex.EncodeToString(hashBytes)

	return base64MD5, hexMD5, nil
}
