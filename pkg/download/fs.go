// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath 灏嗙粰瀹氱殑璺緞 p 鐩稿浜?rootDir 杩涜瑙ｆ瀽銆?//   - 濡傛灉 rootDir 涓虹┖锛岀洿鎺ヨ繑鍥?p銆?//   - 濡傛灉 p 鏄粷瀵硅矾寰勪笖鍦?rootDir 鍐咃紝鍘熸牱杩斿洖銆?//   - 濡傛灉 p 鏄浉瀵硅矾寰勶紝涓?rootDir 鎷兼帴鍚庤繑鍥炪€?//   - 浠讳綍璇曞浘閫冮€?rootDir 鐨勮涓哄潎杩斿洖閿欒銆?func ResolvePath(rootDir, p string) (string, error) {
	if rootDir == "" {
		return p, nil
	}
	if filepath.IsAbs(p) {
		if isWithinRoot(rootDir, p) {
			return p, nil
		}
		return "", fmt.Errorf("path outside root: %s", p)
	}
	rp, err := cleanJoin(rootDir, p)
	if err != nil {
		return "", err
	}
	if !isWithinRoot(rootDir, rp) {
		return "", fmt.Errorf("path outside root: %s", p)
	}
	return rp, nil
}

// ResolvePathWithAllowList 鍦?ResolvePath 鍩虹涓婂鍔犵櫧鍚嶅崟鏍￠獙銆?// 褰?allowPaths 闈炵┖鏃讹紝瑙ｆ瀽鍚庣殑璺緞蹇呴』浣嶄簬鑷冲皯涓€涓櫧鍚嶅崟鐩綍涓嬨€?// 鏈厤缃櫧鍚嶅崟鏃惰涓轰笌 ResolvePath 涓€鑷淬€?func ResolvePathWithAllowList(rootDir string, allowPaths []string, p string) (string, error) {
	resolved, err := ResolvePath(rootDir, p)
	if err != nil {
		return "", err
	}
	if len(allowPaths) == 0 {
		return resolved, nil
	}
	for _, ap := range allowPaths {
		absAP, aErr := filepath.Abs(ap)
		if aErr != nil {
			continue
		}
		if strings.HasPrefix(resolved, absAP+string(filepath.Separator)) || resolved == absAP {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path not in allowed list: %s", p)
}

// isWithinRoot 妫€鏌?p 鏄惁鍦?rootDir 鐨勫畨鍏ㄨ寖鍥村唴銆?func isWithinRoot(rootDir, p string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	if absRoot == absP {
		return true
	}
	if !strings.HasSuffix(absRoot, string(filepath.Separator)) {
		absRoot += string(filepath.Separator)
	}
	return strings.HasPrefix(absP, absRoot)
}

// cleanJoin 灏?rootDir 涓庝换鎰忓厓绱犳嫾鎺ュ苟鐢?filepath.Clean 瑙勮寖鍖栥€?func cleanJoin(rootDir string, elems ...string) (string, error) {
	all := append([]string{rootDir}, elems...)
	return filepath.Clean(filepath.Join(all...)), nil
}

// EnsureDir 纭繚鏂囦欢璺緞鐨勭埗鐩綍瀛樺湪锛堝 MkdirAll锛夈€?func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
