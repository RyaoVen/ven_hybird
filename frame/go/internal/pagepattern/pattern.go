// Package pagepattern 提供页面路由 pattern 的合法性校验。
// pattern 列表由 Node 页面路由权威生成，Go 启动时通过固定请求拉取。
package pagepattern

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// patternsPath 是 Node 端提供页面路由模式列表的固定路径。
const patternsPath = "/pages"

// patternList 是 Node 端 GET /pages 的响应结构。
type patternList struct {
	Patterns []string `json:"patterns"`
}

// Validator 校验页面 pattern 是否存在于 Node 页面路由权威列表中。
type Validator struct {
	patterns map[string]struct{}
}

// NewValidator 用 pattern 列表创建校验器。
func NewValidator(patterns []string) *Validator {
	set := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		set[pattern] = struct{}{}
	}
	return &Validator{patterns: set}
}

// Validate 校验 pattern 是否存在于 Node 页面路由权威列表中。
func (v *Validator) Validate(pattern string) error {
	if _, ok := v.patterns[pattern]; !ok {
		return fmt.Errorf("page pattern not found in node pages: %s", pattern)
	}
	return nil
}

// Patterns 返回去重排序后的 pattern 列表（持久化与诊断用）。
func (v *Validator) Patterns() []string {
	list := make([]string, 0, len(v.patterns))
	for pattern := range v.patterns {
		list = append(list, pattern)
	}
	sort.Strings(list)
	return list
}

// Save 把校验器持久化到磁盘（原子写：临时文件 + rename）。
// 供高可用启动回退使用：Node 不可达时用最近一次成功拉取的 pattern 启动。
// 失败返回 error，调用方按 best-effort 处理（不影响主流程）。
func Save(v *Validator, path string) error {
	if path == "" {
		return fmt.Errorf("page patterns file path is empty")
	}
	data, err := json.Marshal(patternList{Patterns: v.Patterns()})
	if err != nil {
		return fmt.Errorf("encode page patterns: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create patterns dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write page patterns: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("persist page patterns: %w", err)
	}
	return nil
}

// Load 从磁盘加载持久化的校验器（格式与 Save 一致）。
// 文件缺失或内容损坏返回 error，调用方据此决定是否继续失败退出。
func Load(path string) (*Validator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read persisted page patterns: %w", err)
	}
	var list patternList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode persisted page patterns: %w", err)
	}
	return NewValidator(list.Patterns), nil
}

// Fetch 从 Node 工作节点拉取全部页面路由模式并构建校验器。
// 请求为固定形式：GET {nodeWorkerURL}/pages，携带内部令牌。
func Fetch(ctx context.Context, nodeWorkerURL, internalToken string, timeout time.Duration) (*Validator, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, strings.TrimRight(nodeWorkerURL, "/")+patternsPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create page patterns request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if internalToken != "" {
		request.Header.Set("X-Ven-Internal-Token", internalToken)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch page patterns: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		// 限制读取量（最多 4KB），防止恶意响应体导致内存问题
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("fetch page patterns: worker returned %d: %s",
			response.StatusCode, strings.TrimSpace(string(message)))
	}

	var list patternList
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode page patterns: %w", err)
	}
	return NewValidator(list.Patterns), nil
}
