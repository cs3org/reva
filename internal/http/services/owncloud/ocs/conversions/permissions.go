package conversions

import "fmt"

// Permissions reflects the CRUD permissions used in the OCS sharing API
type Permissions uint

const (
	// PermissionInvalid grants no permissions on a resource
	PermissionInvalid Permissions = 0
	// PermissionRead grants read permissions on a resource
	PermissionRead Permissions = 1 << (iota - 1)
	// PermissionWrite grants write permissions on a resource
	PermissionWrite
	// PermissionCreate grants create permissions on a resource
	PermissionCreate
	// PermissionDelete grants delete permissions on a resource
	PermissionDelete
	// PermissionShare grants share permissions on a resource
	PermissionShare
	// PermissionAll grants all permissions on a resource
	PermissionAll Permissions = (1 << (iota - 1)) - 1
)

// NewPermissions creates a new Permissions instanz.
// The value must be in the valid range.
func NewPermissions(val int) (Permissions, error) {
	if val <= int(PermissionInvalid) || int(PermissionAll) < val {
		return PermissionInvalid, fmt.Errorf("permissions %d out of range %d - %d", val, PermissionRead, PermissionAll)
	}
	return Permissions(val), nil
}

// Contain tests if the permissions contain another one.
func (p Permissions) Contain(other Permissions) bool {
	return p&other != 0
}

// Permissions2Role performs permission conversions
func Permissions2Role(p Permissions) string {
	role := RoleLegacy
	if p.Contain(PermissionRead) {
		role = RoleViewer
	}
	if p.Contain(PermissionWrite) {
		role = RoleEditor
	}
	if p.Contain(PermissionShare) {
		role = RoleCoowner
	}
	return role
}
