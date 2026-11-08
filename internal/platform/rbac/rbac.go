package rbac

// 后台角色与权限定义。权限以点号命名，路由层按权限校验。

type Permission struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type Role struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

var permissionList = []Permission{
	{Key: "dashboard.view", Name: "查看数据看板", Category: "概览"},
	{Key: "moderation.review", Name: "内容审核与举报处置", Category: "内容"},
	{Key: "creators.review", Name: "音乐人认证审核", Category: "内容"},
	{Key: "works.manage", Name: "作品下架 / 恢复 / 置顶", Category: "内容"},
	{Key: "users.manage", Name: "用户警告 / 限制 / 封禁", Category: "用户"},
	{Key: "recommend.publish", Name: "推荐策略与名单配置", Category: "运营"},
	{Key: "plaza.manage", Name: "广场运营位与话题", Category: "运营"},
	{Key: "messages.manage", Name: "公告与消息模板", Category: "运营"},
	{Key: "credits.manage", Name: "发起积分调账", Category: "积分"},
	{Key: "credits.review", Name: "积分调账复核", Category: "积分"},
	{Key: "audit.view", Name: "审计日志检索", Category: "系统"},
	{Key: "roles.manage", Name: "角色权限调整", Category: "系统"},
}

var roleList = []Role{
	{
		Key:         "admin",
		Name:        "超级管理员",
		Description: "全量权限，可发布推荐策略与调整权限；至少保留 1 个管理员账号。",
		Permissions: allKeys(),
	},
	{
		Key:         "reviewer",
		Name:        "内容审核员",
		Description: "内容审核、音乐人认证、作品与用户处置；不可调整推荐策略与积分复核。",
		Permissions: []string{"dashboard.view", "moderation.review", "creators.review", "works.manage", "users.manage", "audit.view"},
	},
	{
		Key:         "operator",
		Name:        "运营",
		Description: "看板、作品、推荐、广场、消息与积分发起；不可审核内容、封禁用户或复核积分。",
		Permissions: []string{"dashboard.view", "works.manage", "recommend.publish", "plaza.manage", "messages.manage", "credits.manage", "audit.view"},
	},
	{
		Key:         "user",
		Name:        "普通用户",
		Description: "前台用户，无后台权限。",
		Permissions: []string{},
	},
}

func allKeys() []string {
	out := make([]string, 0, len(permissionList))
	for _, p := range permissionList {
		out = append(out, p.Key)
	}
	return out
}

func Permissions() []Permission { return permissionList }

func Roles() []Role { return roleList }

func roleOf(key string) *Role {
	for i := range roleList {
		if roleList[i].Key == key {
			return &roleList[i]
		}
	}
	return nil
}

func IsKnown(role string) bool { return roleOf(role) != nil }

// IsStaff 表示该角色可以进入后台（拥有至少一个后台权限）。
func IsStaff(role string) bool {
	r := roleOf(role)
	return r != nil && len(r.Permissions) > 0
}

func Allows(role, permission string) bool {
	r := roleOf(role)
	if r == nil {
		return false
	}
	for _, p := range r.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

func PermissionsFor(role string) []string {
	if r := roleOf(role); r != nil {
		return r.Permissions
	}
	return []string{}
}

func RoleName(role string) string {
	if r := roleOf(role); r != nil {
		return r.Name
	}
	return role
}

// Matrix 返回 角色 -> 权限 -> 是否允许 的矩阵，供后台权限页渲染。
func Matrix() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, r := range roleList {
		row := map[string]bool{}
		for _, p := range permissionList {
			row[p.Key] = false
		}
		for _, key := range r.Permissions {
			row[key] = true
		}
		out[r.Key] = row
	}
	return out
}
