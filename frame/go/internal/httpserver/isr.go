// 静态页 ISR 接线：声明注册、失效（DataChange 删除阶段）、物化与上限治理。
package httpserver

import (
	"fmt"
	"log"

	"ven_hybird/internal/isr"
	"ven_hybird/internal/pagecache"
)

// RegisterStaticPage 注册一个静态页声明：pattern 合法性校验 + 模板查重。
func (s *Server) RegisterStaticPage(decl *isr.Declaration) error {
	if err := s.ValidatePagePattern(decl.Template); err != nil {
		return err
	}
	if _, exists := s.staticDecls[decl.Template]; exists {
		return fmt.Errorf("static page already registered: %s", decl.Template)
	}
	s.staticDecls[decl.Template] = decl
	return nil
}

// StaticDecl 返回模板对应的静态页声明（hybrid 预渲染取声明用）。
func (s *Server) StaticDecl(template string) *isr.Declaration {
	return s.staticDecls[template]
}

// StaticFileExists 报告路径是否已有物化文件（诊断/测试用）。
func (s *Server) StaticFileExists(path string) bool {
	return s.isrStore.Exists(path)
}

// staticDeclFor 返回与实际路径匹配的静态页声明及提取的参数。
func (s *Server) staticDeclFor(path string) (*isr.Declaration, map[string]string) {
	for _, decl := range s.staticDecls {
		if params, ok := decl.Match(path); ok {
			return decl, params
		}
	}
	return nil, nil
}

// InvalidateStatic 是事件总线 ① 删除阶段的实现：
// 构造匹配器 → 删除物化文件 → 同步清内存缓存 → 返回删除路径与 smartLoad 热门路径（删除前统计）。
// 由事件总线在静默窗口后调用，不再被 DataChange 直接同步触发。
func (s *Server) InvalidateStatic(template string, params []string) (deleted, hot []string, err error) {
	decl, ok := s.staticDecls[template]
	if !ok {
		return nil, nil, fmt.Errorf("static page not declared: %s", template)
	}
	matcher, err := decl.BuildMatcher(params)
	if err != nil {
		return nil, nil, err
	}
	// smartLoad 全局更新：先按访问统计取热门（统计随删除清零）
	if len(params) == 0 && decl.SmartLoad && decl.MaxPages > 0 {
		hot = s.isrStore.HotPaths(matcher, decl.MaxPages)
	}
	deleted, err = s.isrStore.Invalidate(matcher)
	if err != nil {
		return nil, nil, err
	}
	for _, path := range deleted {
		s.InvalidatePage(path)
	}
	log.Printf("isr: invalidated %d files for %s (params=%v)", len(deleted), template, params)
	return deleted, hot, nil
}

// RenderStaticHTML 回源渲染具体路径并返回 HTML（不落盘，不走页面缓存直回源）。
// 事件总线 ② 再生阶段用：渲染与落盘分离，落盘前由总线做跨代（stale）检查。
func (s *Server) RenderStaticHTML(path string, data any) (string, error) {
	entry, err := s.renderRoute(path, map[string]string{}, data)
	if err != nil {
		return "", err
	}
	if entry == nil || entry.HTML == "" {
		return "", fmt.Errorf("render %s returned empty html", path)
	}
	return entry.HTML, nil
}

// MaterializeStatic 落盘物化文件并按声明治理上限（事件总线 ② 落盘阶段用）。
func (s *Server) MaterializeStatic(path string, html string) error {
	return s.materialize(path, html)
}

// materialize 落盘并按声明治理上限；未声明路径直接跳过。
func (s *Server) materialize(path string, html string) error {
	decl, _ := s.staticDeclFor(path)
	if decl == nil {
		return nil
	}
	if decl.MaxPages > 0 && decl.SmartLoad && !s.isrStore.Exists(path) {
		// smartLoad：已达上限的路径不再写入，超出部分走 SSR
		matcher, err := decl.BuildMatcher(nil)
		if err == nil && s.isrStore.CountFiles(matcher) >= decl.MaxPages {
			return nil
		}
	}
	if err := s.isrStore.Materialize(path, html); err != nil {
		return err
	}
	if decl.MaxPages > 0 && !decl.SmartLoad {
		// 懒删除（LRU）：超过上限时淘汰最久未访问的文件
		matcher, err := decl.BuildMatcher(nil)
		if err == nil {
			s.isrStore.EvictLRU(matcher, decl.MaxPages)
		}
	}
	return nil
}

// materializeQuiet 包装 materialize，失败仅记录日志（物化失败不影响响应）。
func (s *Server) materializeQuiet(path string, html string) {
	if err := s.materialize(path, html); err != nil {
		log.Printf("isr: materialize %s failed: %v", path, err)
	}
}

// renderRoute 回源渲染（render 的无 ctx 版本）：提交 SSR 任务并等待回调。
func (s *Server) renderRoute(route string, query map[string]string, data any) (*pagecache.Entry, error) {
	return s.renderWithQuery(route, query, data)
}
