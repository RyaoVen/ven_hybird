// Package auth 提供权限等级注册与校验能力。
package auth

import "errors"

// Registry 管理权限等级及其继承关系。
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

// Resolve 把一组角色名解析为页面所需的等级列表（取并集）。
// 若传入的角色中存在未注册的角色，返回错误。
func (r *Registry) Resolve(roles []string) ([]int64, error) {
	resolved := make(map[int64]struct{})
	for _, role := range roles {
		levels, ok := r.auths[role]
		if !ok {
			return nil, errors.New("未注册的角色：" + role)
		}
		for _, lv := range levels {
			resolved[lv] = struct{}{}
		}
	}
	out := make([]int64, 0, len(resolved))
	for lv := range resolved {
		out = append(out, lv)
	}
	return out, nil
}
