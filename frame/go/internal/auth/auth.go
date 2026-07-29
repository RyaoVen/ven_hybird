// Package auth 提供权限等级注册与校验能力。
package auth

import "errors"

// Registry 管理权限等级及其继承关系。
// auths[role] 恒为 [自身等级, 祖先等级...]（Register 先存自身再展开祖先闭包）。
type Registry struct {
	auths      map[string][]int64
	authLevels int64
}

// NewRegistry 创建空的权限注册表。
func NewRegistry() *Registry {
	return &Registry{
		auths: make(map[string][]int64),
	}
}

// Register 注册一个权限等级，并可指定继承的父级。
func (r *Registry) Register(level string, fatherLevels []string) error {
	r.auths[level] = append(r.auths[level], r.authLevels)
	r.authLevels++
	if len(fatherLevels) != 0 {
		for _, father := range fatherLevels {
			levels, ok := r.auths[father]
			if !ok {
				return errors.New("不存在该父权级：" + father)
			}
			r.auths[level] = append(r.auths[level], levels...)
		}
	}
	return nil
}

// Check 检查某个权限等级是否命中页面所需的任意等级。
// level 的等级集 = 自身 + 祖先闭包（Register 时展开），pageLevels 为页面声明角色的
// 自身等级（Resolve 产出）：交集命中 = 用户 is-a 页面声明角色。
func (r *Registry) Check(level string, pageLevels []int64) (bool, error) {
	levels, ok := r.auths[level]
	if !ok {
		return false, errors.New("不存在该权级：" + level)
	}
	for _, v1 := range levels {
		for _, v2 := range pageLevels {
			if v1 == v2 {
				return true, nil
			}
		}
	}
	return false, nil
}

// Resolve 把一组角色名解析为页面所需的等级列表（每角色只取自身等级，不展开祖先）。
// 配合 Check 的交集语义实现 RBAC 继承：用户角色的等级闭包（自身+祖先）含任一页面
// 声明角色即放行——"用户 is-a 声明角色"。子角色可访问父角色的页面，父角色不能访问
// 子角色的页面（页面侧若也展开祖先，父子会因共享祖先而互相穿透）。
// 若传入的角色中存在未注册的角色，返回错误。
func (r *Registry) Resolve(roles []string) ([]int64, error) {
	out := make([]int64, 0, len(roles))
	for _, role := range roles {
		levels, ok := r.auths[role]
		if !ok {
			return nil, errors.New("未注册的角色：" + role)
		}
		out = append(out, levels[0]) // levels[0] 恒为自身等级
	}
	return out, nil
}
