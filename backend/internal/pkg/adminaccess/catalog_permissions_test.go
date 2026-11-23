package adminaccess

import "testing"

func TestScopedPermissionCatalogRequiresExplicitGrant(t *testing.T) {
	for _, id := range []string{"accounts.identity.read", "accounts.usage.read", "accounts.options.write", "accounts.rate.write", "groups.list.read", "groups.capacity.read", "openai.tokens.refresh"} {
		found := false
		for _, entry := range PermissionCatalog() {
			if entry.ID == id {
				found = true
			}
		}
		if !found {
			t.Errorf("权限未登记: %s", id)
		}
	}
	if RouteAllowed(Policy{}, "POST", "/api/v1/admin/openai/refresh-token") {
		t.Fatal("旧策略不得继承新增权限")
	}
}

func TestQuotaMultiplierRoutesAreRegistered(t *testing.T) {
	groupsRead := Policy{GlobalPermissions: []string{"groups.read"}}
	if !RouteAllowed(groupsRead, "GET", "/api/v1/admin/groups/:id/quota-multipliers") {
		t.Fatal("groups.read 应允许读取分组额度倍率")
	}

	assign := Policy{GlobalPermissions: []string{"subscriptions.assign"}}
	for _, route := range []struct{ method, path string }{
		{"PUT", "/api/v1/admin/subscriptions/:id/quota-multiplier"},
		{"PUT", "/api/v1/admin/groups/:id/quota-multipliers"},
	} {
		if !RouteAllowed(assign, route.method, route.path) {
			t.Errorf("subscriptions.assign 应允许 %s %s", route.method, route.path)
		}
	}

	// 未授予对应权限时仍然拒绝，避免新增路由被默认放行。
	if RouteAllowed(Policy{}, "GET", "/api/v1/admin/groups/:id/quota-multipliers") {
		t.Fatal("未授权策略不得读取分组额度倍率")
	}
	if RouteAllowed(groupsRead, "PUT", "/api/v1/admin/subscriptions/:id/quota-multiplier") {
		t.Fatal("groups.read 不得修改订阅额度倍率")
	}
}
