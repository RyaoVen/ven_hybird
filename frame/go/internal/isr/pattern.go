// Package isr 提供静态页 ISR：渲染产物落盘、fiber 静态直发、显式失效与淘汰。
package isr

import (
	"fmt"
	"strings"
)

// Declaration 是一个 StaticPage 声明：路由模式 + 上限 + 智能加载开关。
type Declaration struct {
	Template  string   // 完整路由模式，如 /blog/:id、/:user/blog/:id、/about
	Prefix    string   // 首个动态段之前的静态前缀（/blog 或 /）；纯静态页为模式本身
	Segments  []string // 分段后的路由模式（保留 : 前缀）
	MaxPages  int      // 物化文件数上限（0 = 不限）
	SmartLoad bool     // 智能加载：全局更新时按热门预渲染
}

// ParseDeclaration 解析并校验 StaticPage 声明。
// maxPages < 0 视为 0（不限）。
func ParseDeclaration(template string, maxPages int, smartLoad bool) (*Declaration, error) {
	segments, err := splitRoute(template)
	if err != nil {
		return nil, err
	}
	if maxPages < 0 {
		maxPages = 0
	}
	return &Declaration{
		Template:  template,
		Prefix:    derivePrefix(segments),
		Segments:  segments,
		MaxPages:  maxPages,
		SmartLoad: smartLoad,
	}, nil
}

// DynamicCount 返回声明中动态段的数量。
func (d *Declaration) DynamicCount() int {
	count := 0
	for _, segment := range d.Segments {
		if strings.HasPrefix(segment, ":") {
			count++
		}
	}
	return count
}

// Match 判断实际路径是否与声明模式匹配（逐段，: 段匹配任意单段）。
// 匹配时提取动态段参数。
func (d *Declaration) Match(path string) (map[string]string, bool) {
	parts, err := splitRoute(path)
	if err != nil || len(parts) != len(d.Segments) {
		return nil, false
	}
	params := make(map[string]string)
	for index, segment := range d.Segments {
		if strings.HasPrefix(segment, ":") {
			params[segment[1:]] = parts[index]
			continue
		}
		if segment != parts[index] {
			return nil, false
		}
	}
	return params, true
}

// splitRoute 规范化并拆分路径为段。
func splitRoute(route string) ([]string, error) {
	pathname := strings.SplitN(route, "?", 2)[0]
	if pathname == "" {
		pathname = "/"
	}
	normalized := "/" + strings.Trim(pathname, "/")
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		normalized = "/"
	}
	for _, segment := range strings.Split(strings.Trim(normalized, "/"), "/") {
		if segment == ".." || segment == "." {
			return nil, fmt.Errorf("isr: illegal route segment: %s", route)
		}
	}
	if normalized == "/" {
		return []string{}, nil
	}
	return strings.Split(strings.Trim(normalized, "/"), "/"), nil
}

// derivePrefix 推导首个动态段之前的静态前缀。
func derivePrefix(segments []string) string {
	static := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			break
		}
		static = append(static, segment)
	}
	if len(static) == 0 {
		return "/"
	}
	return "/" + strings.Join(static, "/")
}

// Matcher 是 DataChange 推导出的失效范围匹配器。
type Matcher struct {
	decl      *Declaration
	concrete  map[int]string // 已具体化的段位置 → 段值
	exact     bool           // 参数给满，仅匹配单个具体路径
	exactPath string         // exact 时的具体路径
}

// BuildMatcher 按"params 左到右连续填充、尾段缺失即子树"构造匹配器。
// params 个数超过动态段数时报错。
func (d *Declaration) BuildMatcher(params []string) (*Matcher, error) {
	if len(params) > d.DynamicCount() {
		return nil, fmt.Errorf("isr: too many params for %s: got %d, want <= %d",
			d.Template, len(params), d.DynamicCount())
	}
	matcher := &Matcher{decl: d, concrete: make(map[int]string)}
	paramIndex := 0
	for index, segment := range d.Segments {
		if strings.HasPrefix(segment, ":") && paramIndex < len(params) {
			matcher.concrete[index] = params[paramIndex]
			paramIndex++
		}
	}
	// 参数给满动态段 = 仅匹配单个具体路径；否则为子树/全局范围
	matcher.exact = len(params) == d.DynamicCount()
	if matcher.exact {
		matcher.exactPath = d.BuildPath(params)
	}
	return matcher, nil
}

// BuildPath 用 params 按段填充出具体路径（params 个数必须等于动态段数）。
func (d *Declaration) BuildPath(params []string) string {
	parts := make([]string, len(d.Segments))
	paramIndex := 0
	for index, segment := range d.Segments {
		if strings.HasPrefix(segment, ":") {
			if paramIndex < len(params) {
				parts[index] = params[paramIndex]
				paramIndex++
			} else {
				parts[index] = segment
			}
		} else {
			parts[index] = segment
		}
	}
	return "/" + strings.Join(parts, "/")
}

// Match 判断实际路径是否落在匹配器范围内。
func (m *Matcher) Match(path string) bool {
	if m.exact {
		return normalizePath(path) == m.exactPath
	}
	parts, err := splitRoute(path)
	if err != nil || len(parts) != len(m.decl.Segments) {
		return false
	}
	for index, segment := range m.decl.Segments {
		if concrete, ok := m.concrete[index]; ok && parts[index] != concrete {
			return false
		}
		if !strings.HasPrefix(segment, ":") && segment != parts[index] {
			return false
		}
	}
	return true
}

// normalizePath 规范化路径（去尾部斜杠，保留根 /）。
func normalizePath(path string) string {
	pathname := strings.SplitN(path, "?", 2)[0]
	if pathname == "" || pathname == "/" {
		return "/"
	}
	return "/" + strings.Trim(pathname, "/")
}
