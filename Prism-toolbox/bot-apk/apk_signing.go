package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
)

// apkSigningCertHash 从 APK 文件解析 APK Signature Scheme v2/v3 签名块，
// 提取签名证书（DER 编码的 X.509）并返回其 SHA-256（hex 小写）。
//
// 关键点：此函数**直接读 APK 文件本身**，不信任 Java 上报的任何值。
// 任何重打包/改签名都必须修改 APK 的签名块 → 签名证书必变 → 哈希不匹配白名单。
// 纯标准库实现，不依赖第三方库。
func apkSigningCertHash(apkPath string) (string, error) {
	data, err := os.ReadFile(apkPath)
	if err != nil {
		return "", fmt.Errorf("read apk: %w", err)
	}
	if len(data) < 22 {
		return "", fmt.Errorf("apk too small")
	}

	// 1. 从文件尾部向前找 EOCD 签名 "PK\x05\x06"
	eocd := -1
	for i := len(data) - 22; i >= 0; i-- {
		if data[i] == 'P' && data[i+1] == 'K' && data[i+2] == 0x05 && data[i+3] == 0x06 {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return "", fmt.Errorf("no EOCD found")
	}
	// EOCD 偏移 +16 处是 central directory 偏移 (u32)
	cdOffset := int(binary.LittleEndian.Uint32(data[eocd+16 : eocd+20]))

	// 2. 签名块紧挨在 central directory 之前，其尾部固定 24 字节：
	//    [8字节 size][16字节 "APK Sig Block 42"]
	const magic = "APK Sig Block 42"
	tail := cdOffset - 24
	if tail < 0 {
		return "", fmt.Errorf("no room for signing block")
	}
	if string(data[tail+8:tail+24]) != magic {
		return "", fmt.Errorf("no APK Sig Block magic (not v2/v3 signed?)")
	}
	blockSize := int(binary.LittleEndian.Uint64(data[tail : tail+8]))
	// 签名块起点 = cdOffset - 24 - blockSize；块结束于 cdOffset-8（结尾 size 字段之前）。
	// 实测：块内真正的 ID-value pair 从偏移 24 开始。
	blockStart := cdOffset - 24 - blockSize
	if blockStart < 0 {
		return "", fmt.Errorf("bad signing block size")
	}
	block := data[blockStart : cdOffset-8]

	// 3. 遍历签名块内的 ID-value 对，找 ID=0x7109871a（APK Signature Scheme v2）。
	//    块开头是 8 字节 size + 16 字节 magic，真正的 pair 从偏移 24 开始。
	pos := 24
	for pos+8 <= len(block) {
		pairSize := int(binary.LittleEndian.Uint64(block[pos : pos+8]))
		if pairSize < 4 || pos+8+pairSize > len(block) {
			break
		}
		pair := block[pos+8 : pos+8+pairSize]
		id := binary.LittleEndian.Uint32(pair[:4])
		if id == 0x7109871a {
			return v2BlockCertHash(pair[4:])
		}
		pos += 8 + pairSize
	}
	return "", fmt.Errorf("no v2 signer block in signing block")
}

// v2BlockCertHash 解析 v2 块 value（ID 0x7109871a）：
//   value = [u32 序列长度][signer1][signer2]...   每个 signer 是长度前缀
//   signer = [signedData][signatures][publicKey]（均为长度前缀）
//   signedData = [digests][certificates][attributes]（证书在 signedData 内部）
// 取第一个 signer 的 signedData 里第一个证书。
func v2BlockCertHash(value []byte) (string, error) {
	// 长度前缀切片：读 u32 长度，返回内容切片
	lenPref := func(b []byte) ([]byte, error) {
		if len(b) < 4 {
			return nil, fmt.Errorf("truncated length prefix")
		}
		n := int(binary.LittleEndian.Uint32(b[:4]))
		if n < 0 || 4+n > len(b) {
			return nil, fmt.Errorf("bad length prefix %d (len=%d)", n, len(b))
		}
		return b[4 : 4+n], nil
	}
	// 长度前缀 + 跳过该字段，返回切片
	skipField := func(b []byte, pos *int) ([]byte, error) {
		if *pos+4 > len(b) {
			return nil, fmt.Errorf("field truncated")
		}
		n := int(binary.LittleEndian.Uint32(b[*pos : *pos+4]))
		*pos += 4
		if n < 0 || *pos+n > len(b) {
			return nil, fmt.Errorf("field out of range")
		}
		sl := b[*pos : *pos+n]
		*pos += n
		return sl, nil
	}

	// 1. v2 块 value → signer 序列 → 第一个 signer
	seq, err := lenPref(value)
	if err != nil {
		return "", fmt.Errorf("signers sequence: %w", err)
	}
	signer, err := lenPref(seq)
	if err != nil {
		return "", fmt.Errorf("first signer: %w", err)
	}
	// 2. signer = [signedData][signatures][publicKey]
	sp := 0
	signedData, err := skipField(signer, &sp)
	if err != nil {
		return "", fmt.Errorf("signedData: %w", err)
	}
	if _, err := skipField(signer, &sp); err != nil { // signatures
		return "", fmt.Errorf("signatures: %w", err)
	}
	if _, err := skipField(signer, &sp); err != nil { // publicKey
		return "", fmt.Errorf("publicKey: %w", err)
	}
	// 3. signedData = [digests][certificates][attributes]
	dp := 0
	if _, err := skipField(signedData, &dp); err != nil { // digests
		return "", fmt.Errorf("digests: %w", err)
	}
	certs, err := skipField(signedData, &dp) // certificates
	if err != nil {
		return "", fmt.Errorf("certificates: %w", err)
	}
	// 4. certificates → 第一个证书。实测内容直接是 DER X.509（0x30 开头）；
	//    个别实现用长度前缀序列，做兼容。
	var certDER []byte
	if len(certs) > 0 && certs[0] == 0x30 {
		certDER = certs
	} else {
		certDER, err = lenPref(certs)
		if err != nil {
			return "", fmt.Errorf("first certificate: %w", err)
		}
	}
	if _, err := x509.ParseCertificate(certDER); err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	h := sha256.Sum256(certDER)
	return hex.EncodeToString(h[:]), nil
}
