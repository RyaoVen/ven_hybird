package auth

import "testing"

// 注册继承链：reader ← author ← admin，兄弟 editor ← reader。
func newInheritedRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	for _, entry := range []struct {
		role    string
		parents []string
	}{
		{"reader", nil},
		{"author", []string{"reader"}},
		{"admin", []string{"author"}},
		{"editor", []string{"reader"}},
	} {
		if err := r.Register(entry.role, entry.parents); err != nil {
			t.Fatalf("register %s failed: %v", entry.role, err)
		}
	}
	return r
}

func mustResolve(t *testing.T, r *Registry, roles ...string) []int64 {
	t.Helper()
	levels, err := r.Resolve(roles)
	if err != nil {
		t.Fatalf("resolve %v failed: %v", roles, err)
	}
	return levels
}

func check(t *testing.T, r *Registry, role string, pageLevels []int64) bool {
	t.Helper()
	allowed, err := r.Check(role, pageLevels)
	if err != nil {
		t.Fatalf("check %s failed: %v", role, err)
	}
	return allowed
}

// 回归：页面声明子角色时，父角色不得命中（继承守卫穿透）。
func TestRegistry_ParentCannotAccessChildPage(t *testing.T) {
	r := newInheritedRegistry(t)
	page := mustResolve(t, r, "author")

	if !check(t, r, "author", page) {
		t.Error("author should access author page")
	}
	if check(t, r, "reader", page) {
		t.Error("reader must NOT access author page (guard penetration)")
	}
	if !check(t, r, "admin", page) {
		t.Error("admin (descendant of author) should access author page")
	}
}

func TestRegistry_ChildAccessesParentPage(t *testing.T) {
	r := newInheritedRegistry(t)
	page := mustResolve(t, r, "reader")

	for _, role := range []string{"reader", "author", "admin", "editor"} {
		if !check(t, r, role, page) {
			t.Errorf("%s should access reader page", role)
		}
	}
}

func TestRegistry_SiblingsIsolated(t *testing.T) {
	r := newInheritedRegistry(t)
	authorPage := mustResolve(t, r, "author")
	editorPage := mustResolve(t, r, "editor")

	if check(t, r, "editor", authorPage) {
		t.Error("editor must NOT access author page (sibling)")
	}
	if check(t, r, "author", editorPage) {
		t.Error("author must NOT access editor page (sibling)")
	}
}

func TestRegistry_TransitiveInheritance(t *testing.T) {
	r := newInheritedRegistry(t)
	adminPage := mustResolve(t, r, "admin")

	if !check(t, r, "admin", adminPage) {
		t.Error("admin should access admin page")
	}
	if check(t, r, "author", adminPage) {
		t.Error("author must NOT access admin page (ancestor)")
	}
	if check(t, r, "reader", adminPage) {
		t.Error("reader must NOT access admin page (ancestor)")
	}
}

func TestRegistry_MultiRolePage(t *testing.T) {
	r := newInheritedRegistry(t)
	page := mustResolve(t, r, "author", "editor")

	if !check(t, r, "author", page) || !check(t, r, "editor", page) {
		t.Error("listed roles should hit multi-role page")
	}
	if !check(t, r, "admin", page) {
		t.Error("admin (is-a author) should hit multi-role page")
	}
	if check(t, r, "reader", page) {
		t.Error("reader must NOT hit author/editor page")
	}
}

func TestRegistry_Errors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("orphan", []string{"ghost"}); err == nil {
		t.Error("expected error for unknown parent role")
	}
	if _, err := r.Resolve([]string{"ghost"}); err == nil {
		t.Error("expected error for unregistered role in Resolve")
	}
	if _, err := r.Check("ghost", []int64{0}); err == nil {
		t.Error("expected error for unregistered role in Check")
	}
}
