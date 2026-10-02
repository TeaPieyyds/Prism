package pluginscan

import (
	"regexp"
	"strings"

	"github.com/yuin/gopher-lua"
)

// CheckLuaSyntax 校验 Lua 源码语法是否正确（用 gopher-lua 解析，只编译不执行，安全）。
// 语法错误返回 error。用于上传插件时拒绝语法错误的 main.lua。
func CheckLuaSyntax(src string) error {
	L := lua.NewState()
	defer L.Close()
	if _, err := L.LoadString(src); err != nil {
		return err
	}
	return nil
}

// pythonFeatureRe 在剥离注释/字符串后，检测 Python 独有语法特征。
// 命中多个特征即判定该"Lua"实为 Python 代码（或 Python 转 Lua 失败半成品），拒绝上传。
var pythonFeatureChecks = []struct{ name, re string }{
	{"def 函数定义", `(?m)^\s*def\s+[A-Za-z_]\w*\s*\(`},
	{"import 导入", `(?m)^\s*import\s+[A-Za-z_]\w*`},
	{"from…import 导入", `(?m)^\s*from\s+[A-Za-z_]\w*\s+import\s`},
	{"class 类定义", `(?m)^\s*class\s+[A-Za-z_]\w*\s*[:(]`},
	{"threading 线程", `(?m)^\s*import\s+threading\b|\bthreading\.`},
	{"os.path 路径", `\bos\.path\b`},
	{"raise 抛异常", `\braise\s+[A-Za-z_]\w*\s*\(`},
	{"yield 生成器", `\byield\b`},
	{"列表推导式", `\]\s*for\s+[A-Za-z_]\w*\s+in\b`},
	{"match/case 模式匹配", `(?m)^\s*match\s+[A-Za-z_]\w*\s*:`},
	{"lambda 匿名函数", `\blambda\s+[A-Za-z_]\w*\s*:`},
	{"Python 切片", `[A-Za-z_]\w*\[\s*-?\d+\s*:\s*-?\d+\]`},
	{"None 空值", `\bNone\b`},
}

// LooksLikePython 检测源码是否含明显的 Python 语法特征（剥离注释与字符串后）。
// 命中 2 个及以上不同特征即判定为 Python（返回检测到的特征描述），否则返回空串表示正常 Lua。
func LooksLikePython(src string) string {
	s := stripLuaNoise(src)
	var hits []string
	for _, c := range pythonFeatureChecks {
		if regexp.MustCompile(c.re).MatchString(s) {
			hits = append(hits, c.name)
		}
	}
	if len(hits) >= 2 {
		return strings.Join(hits, "、")
	}
	return ""
}
