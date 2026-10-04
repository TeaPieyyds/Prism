package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// 官方合法 APK 签名证书的 SHA-256 集合。
// 被篡改/重打包的 APK 必须用新密钥重签，哈希不在该集合内 → 启动时被硬拦截。
// 注意：开源版本不内置任何签名哈希（生产构建通过注入填充），故留空。
var officialSigHashes = []string{}

// 官方包名。被改包名会在这里被拦。
const officialPkgName = "com.prismtool.box"

// Build-time injected resource hashes.
// Set by: go build -ldflags "-X main.frontendHash=sha256:xxx -X main.soundsHash=sha256:xxx"
var (
	frontendHash = ""
	soundsHash   = ""
	apkSigHash   = "" // set from Android JNI at runtime (Java 上报，作兜底)
	apkPkgName   = "" // set from Android JNI at runtime
	apkPath      = "" // set from Android JNI at runtime（权威校验直接读这个 APK）
)

// 篡改状态（线程安全）。判定为被篡改后，HTTP 层会对所有请求返回拦截页。
var (
	tamperedMu   sync.Mutex
	apkTampered  bool
	tamperReason string
)

// IntegrityResult holds the result of a single integrity check.
type IntegrityResult struct {
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// SetApkSignatureHash stores the APK signature hash received from Android.
func SetApkSignatureHash(hash string) {
	apkSigHash = hash
}

// GetApkSignatureHash returns the stored APK signature hash.
func GetApkSignatureHash() string {
	return apkSigHash
}

// SetApkPackageName stores the APK package name received from Android.
func SetApkPackageName(pkg string) {
	apkPkgName = pkg
}

// SetApkPath stores the installed APK file path received from Android.
// Go 直接读该文件的签名证书做权威校验，不信任 Java 上报的哈希/包名。
func SetApkPath(path string) {
	apkPath = path
}

// hashInWhitelist 判断给定签名哈希是否命中任一官方密钥。
func hashInWhitelist(h string) bool {
	t := strings.TrimSpace(h)
	for _, x := range officialSigHashes {
		if strings.EqualFold(t, strings.TrimSpace(x)) {
			return true
		}
	}
	return false
}

// IsApkTampered reports whether startup verification decided the APK is not official.
func IsApkTampered() bool {
	tamperedMu.Lock()
	defer tamperedMu.Unlock()
	return apkTampered
}

// TamperReason returns the reason the APK was flagged as tampered ("" if clean).
func TamperReason() string {
	tamperedMu.Lock()
	defer tamperedMu.Unlock()
	return tamperReason
}

// MarkTampered flips the tampered flag on (e.g. from a server-authoritative verdict).
func MarkTampered(reason string) {
	tamperedMu.Lock()
	defer tamperedMu.Unlock()
	apkTampered = true
	if reason != "" {
		tamperReason = reason
	}
}

// VerifyApkIntegrity 做权威完整性校验，决定 apkTampered 标志。
// 优先：直接读 APK 文件的签名证书（不信任 Java，破解者改 Java 伪造也绕不过）。
// 兜底：拿不到 APK 路径时，退回 Java 上报的签名/包名校验。
// 在启动时调用一次（GoMain 之前 Java 已通过 JNI 上报 APK 路径/签名/包名）。
func VerifyApkIntegrity() {
	tamperedMu.Lock()
	defer tamperedMu.Unlock()

	// 1. 权威校验：直接读 APK 签名证书。
	if apkPath != "" {
		h, err := apkSigningCertHash(apkPath)
		if err != nil {
			apkTampered = true
			tamperReason = "无法校验 APK 签名（" + err.Error() + "）"
			return
		}
		if !hashInWhitelist(h) {
			apkTampered = true
			tamperReason = "APK 签名证书不匹配，可能已被重打包"
			return
		}
		apkTampered = false
		tamperReason = ""
		return
	}

	// 2. 兜底：Java 上报的签名/包名校验（拿不到 APK 路径时）。
	if strings.TrimSpace(apkSigHash) == "" {
		apkTampered = true
		tamperReason = "未检测到 APK 签名"
		return
	}
	if !hashInWhitelist(apkSigHash) {
		apkTampered = true
		tamperReason = "APK 签名不匹配，可能已被重打包"
		return
	}
	if apkPkgName != "" && !strings.EqualFold(strings.TrimSpace(apkPkgName), officialPkgName) {
		apkTampered = true
		tamperReason = "应用包名被修改"
		return
	}
	apkTampered = false
	tamperReason = ""
}

// RunIntegrityChecks verifies embedded resources haven't been tampered with.
// Returns a map of check name → result.
func RunIntegrityChecks() map[string]IntegrityResult {
	results := make(map[string]IntegrityResult)

	// Check frontend/index.html
	if frontendHash != "" {
		data, err := frontendFS.ReadFile("frontend/index.html")
		if err != nil {
			results["frontend"] = IntegrityResult{Passed: false, Message: "无法读取前端资源: " + err.Error()}
		} else {
			h := sha256.Sum256(data)
			actual := hex.EncodeToString(h[:])
			expected := frontendHash
			if len(expected) > 7 && expected[:7] == "sha256:" {
				expected = expected[7:]
			}
			if actual == expected {
				results["frontend"] = IntegrityResult{Passed: true, Message: "OK"}
			} else {
				results["frontend"] = IntegrityResult{Passed: false, Message: fmt.Sprintf("前端资源被篡改 (期望:%s 实际:%s)", expected[:16], actual[:16])}
			}
		}
	}

	return results
}
