package unmcpk

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

func md5Hex(parts ...string) string {
	h := md5.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func extractS1S2FromSource(code string) (string, string, error) {
	re := regexp.MustCompile(`message = '([^']*)' \+ data \+ '([^']*)'`)
	m := re.FindStringSubmatch(code)
	if len(m) != 3 {
		return "", "", fmt.Errorf("pattern not found")
	}
	return m[1], m[2], nil
}

func extractS1S2FromRepairedByRegex(repaired []byte) (string, string, error) {
	const startIndex = 251
	endIndex := 0
	for i := startIndex; i < len(repaired); i++ {
		if repaired[i] == 0x28 {
			endIndex = i
			break
		}
	}
	if endIndex <= startIndex {
		return "", "", fmt.Errorf("pattern not found")
	}

	target := repaired[startIndex:endIndex]
	re := regexp.MustCompile(`(?s)s.\x00{3}`)
	matches := re.FindIndex(target)
	if matches == nil {
		return "", "", fmt.Errorf("pattern not found")
	}
	s1 := string(target[:matches[0]])
	s2 := string(target[matches[1]:])
	return s1, s2, nil
}

func GenerateTransferCheckNum(isPC bool, data, engineVersion, patchVersion, python3Path string) (string, error) {
	var arr []any
	if err := json.Unmarshal([]byte(data), &arr); err != nil || len(arr) < 3 {
		return "", fmt.Errorf("bad data")
	}

	val, _ := arr[1].(string)
	uniqueFloat, _ := arr[2].(float64)
	uniqueID := int64(uniqueFloat)
	mcpHex, _ := arr[0].(string)
	mcpBytes, err := hex.DecodeString(mcpHex)
	if err != nil {
		return "", fmt.Errorf("bad mcp hex")
	}

	if python3Path == "" {
		python3Path = "python3"
	}

	const useRegex = true
	var s1, s2 string
	if useRegex {
		repaired, err := DecryptDynamicMCP(mcpBytes)
		if err != nil {
			return "", fmt.Errorf("decompile failed")
		}
		s1, s2, err = extractS1S2FromRepairedByRegex(repaired)
		if err != nil {
			return "", fmt.Errorf("pattern not found")
		}
	} else {
		source, err := DecompileDynamicMCP(mcpBytes, python3Path)
		if err != nil {
			return "", fmt.Errorf("decompile failed")
		}
		fmt.Fprintf(os.Stderr, "[CHECKNUM] decompiled MCP source (first 500):\n%s\n", source[:min(500, len(source))])
		s1, s2, err = extractS1S2FromSource(source)
		if err != nil {
			return "", fmt.Errorf("pattern not found")
		}
	}

	fmt.Fprintf(os.Stderr, "[CHECKNUM] s1=%q s2=%q val=%q uid=%d ev=%s pv=%s isPC=%v\n", s1, s2, val, uniqueID, engineVersion, patchVersion, isPC)

	valm := md5Hex(s1, val+"0", s2)
	fmt.Fprintf(os.Stderr, "[CHECKNUM] valm = md5(%q + %q + %q) = %s\n", s1, val+"0", s2, valm)

	tmps := make([]string, 0, len(valm)+7)
	for _, ch := range valm {
		tmps = append(tmps, fmt.Sprintf("%d", ((int(ch))*2+5)^255))
	}
	if !isPC {
		tmps = append(tmps, engineVersion, "android", patchVersion, "android", "2", "12", fmt.Sprintf("%d", uniqueID))
	} else {
		tmps = append(tmps, engineVersion, "windows", engineVersion, "win32", "0", "12", fmt.Sprintf("%d", uniqueID))
	}
	tmpsStr := ""
	for _, s := range tmps {
		tmpsStr += s
	}
	tmpsNum := md5Hex(s1, tmpsStr, s2)
	fmt.Fprintf(os.Stderr, "[CHECKNUM] tmpsStr=%s\n[CHECKNUM] tmpsNum = md5(%q + tmpsStr + %q) = %s\n", tmpsStr[:min(200, len(tmpsStr))], s1, s2, tmpsNum)

	raw := valm[16:] + "False[]3" + tmpsNum + valm[:16]
	sign := md5Hex(s1, raw, s2)
	fmt.Fprintf(os.Stderr, "[CHECKNUM] raw=%s\n[CHECKNUM] sign = md5(%q + raw + %q) = %s\n", raw[:min(200, len(raw))], s1, s2, sign)

	valueJSON := fmt.Sprintf("[\"%s\",\"%s\",false,[],\"\",\"\",3,\"%s\"]", valm, sign, tmpsNum)
	return valueJSON, nil
}

func min(a, b int) int { if a < b { return a }; return b }
