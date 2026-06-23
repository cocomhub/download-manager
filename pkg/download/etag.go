// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"os"
)

// DownloadAction 鍐冲畾鏈涓嬭浇鎿嶄綔鐨勭被鍨嬨€?type DownloadAction int

const (
	// ActionDownload 鍏ㄦ柊涓嬭浇锛堟枃浠朵笉瀛樺湪鎴栨棤娉曢獙璇侊級銆?	ActionDownload DownloadAction = iota
	// ActionResume 鏂偣缁紶锛堟枃浠跺瓨鍦ㄤ笉瀹屾暣锛屾棤 checksum 浣嗘湁 ETag锛夈€?	ActionResume
	// ActionSkip 璺宠繃涓嬭浇锛圗Tag + checksum 涓€鑷达紝鏂囦欢瀹屾暣锛夈€?	ActionSkip
	// ActionReDownload 閲嶆柊涓嬭浇锛圗Tag 涓嶄竴鑷存垨 checksum 涓嶅尮閰嶏級銆?	ActionReDownload
)

// FileStatFunc 鏂囦欢鐘舵€佹煡璇㈡帴鍙ｏ紝鍙敱 os.Stat 鎴?mock 瀹炵幇銆?type FileStatFunc func(path string) (os.FileInfo, error)

// FileChecksumFunc 鏂囦欢鏍￠獙鍜岃绠楁帴鍙ｏ紝鍙敱 ComputeFileMD5 鎴?mock 瀹炵幇銆?type FileChecksumFunc func(path string) (string, error)

// ResolveAction 鏍规嵁鏂囦欢鏄惁瀛樺湪銆丒Tag 鍖归厤鐘舵€佸拰 checksum 鏍￠獙缁撴灉锛?// 鍐冲畾鏈涓嬭浇搴旈噰鍙栦綍绉嶆搷浣滐紙鍏ㄦ柊涓嬭浇/缁紶/璺宠繃/閲嶆柊涓嬭浇锛夈€?//
// 鍐崇瓥鐭╅樀锛?//
//	鏂囦欢瀛樺湪?	ETag 鍖归厤?	Checksum 鍖归厤?	琛屼负
//	鍚?			鈥?			鈥?			ActionDownload
//	瀹屾暣			涓€鑷?		涓€鑷?			ActionSkip
//	瀹屾暣			涓€鑷?		涓嶅瓨鍦?涓嶄竴鑷?	ActionReDownload(鏂囦欢鎹熷潖)
//	涓嶅畬鏁?	涓€鑷?			鈥?			ActionResume
//	瀹屾暣			涓嶄竴鑷?		鈥?			ActionReDownload
//	瀹屾暣			鏃犺褰?鏃ф暟鎹?	瀛樺湪涓斾竴鑷?	ActionSkip(鍏煎鏃ф暟鎹?
//	瀹屾暣			鏃犺褰?鏃ф暟鎹?	涓嶅瓨鍦?		ActionDownload
func ResolveAction(localPath string, prevETag, prevChecksum string, statFunc FileStatFunc, checksumFunc FileChecksumFunc) DownloadAction {
	fi, err := statFunc(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionDownload
		}
		// 鏃犳硶 stat 瑙嗕负涓嶅瓨鍦紝瀹夊叏璧板叏鏂颁笅杞?		return ActionDownload
	}

	fileSize := fi.Size()
	// 鏂囦欢澶у皬涓?0 瑙嗕负涓嶅瓨鍦?	if fileSize == 0 {
		return ActionDownload
	}

	// 鏂囦欢涓嶅畬鏁达紙鏃?checksum 鍙牎楠岋紝璇存槑涓婃鏈畬鎴愪笅杞斤級
	// 濡傛灉 prevETag != ""锛屽皾璇曠画浼狅紝鍚﹀垯閲嶆柊涓嬭浇
	localChecksum, ckErr := checksumFunc(localPath)
	if ckErr != nil && prevETag != "" {
		return ActionResume
	}

	// 鏈?ETag 璁板綍浣嗘棤 checksum 璁板綍锛岃Е鍙戞潯浠惰姹傦紙If-None-Match锛?	// 濡傛灉鏈嶅姟绔繑鍥?304锛岃〃绀烘枃浠舵湭鍙樻洿锛岃烦杩囦笅杞?	if prevETag != "" && prevChecksum == "" {
		// 鏂囦欢瀹屽ソ浣嗘棤涓婃 checksum 璁板綍锛岃蛋鏉′欢璇锋眰璁╂湇鍔＄鍐冲畾
		return ActionDownload
	}

	// 鍏煎鏃ф暟鎹細鏃?ETag 浣嗘湁 checksum 涓斿尮閰嶆枃浠?	if prevETag == "" && prevChecksum != "" && localChecksum == prevChecksum {
		return ActionSkip
	}

	// 鍏煎鏃ф暟鎹細鏃?ETag 涓旀棤 checksum 璁板綍
	if prevETag == "" && prevChecksum == "" {
		return ActionDownload
	}

	// ETag 鍖归厤涓?checksum 鍖归厤 鈫?璺宠繃
	if prevETag != "" && localChecksum == prevChecksum {
		return ActionSkip
	}

	// ETag 鍖归厤浣?checksum 涓嶅尮閰?鈫?鏂囦欢鎹熷潖锛岄渶瑕侀噸涓?	if prevETag != "" && localChecksum != prevChecksum {
		return ActionReDownload
	}

	// ETag 涓嶅尮閰?	return ActionReDownload
}
