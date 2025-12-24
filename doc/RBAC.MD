动态更新 Casbin 权限
你可以通过 *casbin.Enforcer 提供的 API 动态管理策略。

1. 创建权限管理服务
   go
   package auth

// 添加角色的 API 权限
func (s *HzSystemAuthService) AddPermission(ctx context.Context, c *app.RequestContext) {
var req struct {
Role   string `json:"role"`
Path   string `json:"path"`
Method string `json:"method"`
}
if err := c.BindJSON(&req); err != nil {
hresp.ReqBindErr(c, err)
return
}

    // 添加策略
    ok, err := s.Casbin.AddPolicy(req.Role, req.Path, req.Method)
    if err != nil {
        hresp.BaseErrMsg(c, 1, err.Error())
        return
    }
    if !ok {
        hresp.BaseErrMsg(c, 2, "策略已存在")
        return
    }

    hresp.BaseSuccessMsg(c, "添加权限成功")
}

// 删除角色的 API 权限
func (s *HzSystemAuthService) RemovePermission(ctx context.Context, c *app.RequestContext) {
var req struct {
Role   string `json:"role"`
Path   string `json:"path"`
Method string `json:"method"`
}
if err := c.BindJSON(&req); err != nil {
hresp.ReqBindErr(c, err)
return
}

    ok, err := s.Casbin.RemovePolicy(req.Role, req.Path, req.Method)
    if err != nil {
        hresp.BaseErrMsg(c, 1, err.Error())
        return
    }
    if !ok {
        hresp.BaseErrMsg(c, 2, "策略不存在")
        return
    }

    hresp.BaseSuccessMsg(c, "删除权限成功")
}

// 获取角色的所有权限
func (s *HzSystemAuthService) GetPermissions(ctx context.Context, c *app.RequestContext) {
role := c.Query("role")

    // 获取角色的所有策略
    policies := s.Casbin.GetFilteredPolicy(0, role)
    
    hresp.BaseSuccessMsgWithBody(c, "获取成功", policies)
}

// 给用户分配角色
func (s *HzSystemAuthService) AssignRole(ctx context.Context, c *app.RequestContext) {
var req struct {
User string `json:"user"`
Role string `json:"role"`
}
if err := c.BindJSON(&req); err != nil {
hresp.ReqBindErr(c, err)
return
}

    // 添加用户-角色关联 (g = user, role)
    ok, err := s.Casbin.AddGroupingPolicy(req.User, req.Role)
    if err != nil {
        hresp.BaseErrMsg(c, 1, err.Error())
        return
    }
    if !ok {
        hresp.BaseErrMsg(c, 2, "角色已分配")
        return
    }

    hresp.BaseSuccessMsg(c, "分配角色成功")
}
2. 常用 Enforcer API
   方法	作用
   AddPolicy(role, path, method)	添加权限策略
   RemovePolicy(role, path, method)	删除权限策略
   GetFilteredPolicy(0, role)	获取某角色的所有策略
   GetAllRoles()	获取所有角色
   AddGroupingPolicy(user, role)	给用户分配角色
   RemoveGroupingPolicy(user, role)	移除用户角色
   GetRolesForUser(user)	获取用户的所有角色
   GetUsersForRole(role)	获取角色下的所有用户
3. 示例：初始化默认权限
   go
   func InitDefaultPolicies(enforcer *casbin.Enforcer) {
   // 清空旧策略（可选）
   // enforcer.ClearPolicy()

   // 用户角色权限
   enforcer.AddPolicy("user", "/api/user/*", "GET")
   enforcer.AddPolicy("user", "/api/user/profile", "POST")

   // 管理员权限
   enforcer.AddPolicy("admin", "/api/*", "GET")
   enforcer.AddPolicy("admin", "/api/*", "POST")
   enforcer.AddPolicy("admin", "/admin/*", "*")

   // 匿名用户（公开接口）
   enforcer.AddPolicy("", "/auth/*", "POST")
   }
4. 注意事项
   Ent Adapter 自动持久化：调用 AddPolicy / RemovePolicy 后会自动保存到 MySQL
   无需手动 SavePolicy()：使用 Adapter 时自动同步
   实时生效：策略修改后立即生效，无需重启
5. API 路由示例
   go
   // 权限管理路由（仅管理员）
   admin := h.Group("/admin")
   admin.Use(jwtMw.AuthMw, casbinMw.RequiresPermissions())
   {
   admin.POST("/permission/add", authService.AddPermission)
   admin.POST("/permission/remove", authService.RemovePermission)
   admin.GET("/permission/list", authService.GetPermissions)
   admin.POST("/role/assign", authService.AssignRole)
   }