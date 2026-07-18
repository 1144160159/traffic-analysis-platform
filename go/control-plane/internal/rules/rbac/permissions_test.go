package rbac

import "testing"

func TestDeploymentApprovalPermissionSeparation(t *testing.T) {
	if !HasPermission(GetRolePermissions(RoleOperator), PermDeployApprove) {
		t.Fatal("operator must be eligible to approve another user's deployment")
	}
	if HasPermission(GetRolePermissions(RoleAnalyst), PermDeployApprove) {
		t.Fatal("analyst must not receive deployment approval permission")
	}
	if !HasPermission([]Permission{PermDeployAll}, PermDeployApprove) {
		t.Fatal("deploy:* must include deploy:approve")
	}
}
