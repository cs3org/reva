package decomposedfs

import (
	"context"

	cs3permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"google.golang.org/grpc"
)

// PermissionsChecker defines an interface for checking permissions on a Node
type PermissionsChecker interface {
	AssemblePermissions(ctx context.Context, n *node.Node) (ap provider.ResourcePermissions, err error)
}

// CS3PermissionsClient defines an interface for checking permissions against the CS3 permissions service
type CS3PermissionsClient interface {
	CheckPermission(ctx context.Context, in *cs3permissions.CheckPermissionRequest, opts ...grpc.CallOption) (*cs3permissions.CheckPermissionResponse, error)
}

// Permissions manages permissions
type Permissions struct {
	item  PermissionsChecker   // handles item permissions
	space CS3PermissionsClient // handlers space permissions
}

// NewPermissions returns a new Permissions instance
func NewPermissions(item PermissionsChecker, space CS3PermissionsClient) Permissions {
	return Permissions{item: item, space: space}
}

// AssemblePermissions is used to assemble file permissions
func (p Permissions) AssemblePermissions(ctx context.Context, n *node.Node) (provider.ResourcePermissions, error) {
	return p.item.AssemblePermissions(ctx, n)
}

// Manager returns true if the user has the manager role on the space the node belongs to
func (p Permissions) Manager(ctx context.Context, n *node.Node) bool {
	return p.checkRole(ctx, n, "manager")
}

// Editor returns true if the user has the editor role on the space the node belongs to
func (p Permissions) Editor(ctx context.Context, n *node.Node) bool {
	return p.checkRole(ctx, n, "editor")
}

// Viewer returns true if the user has the viewer role on the space the node belongs to
func (p Permissions) Viewer(ctx context.Context, n *node.Node) bool {
	return p.checkRole(ctx, n, "viewer")
}

// CreateSpace returns true when the user is allowed to create the space
func (p Permissions) CreateSpace(ctx context.Context, spaceid string) bool {
	return p.checkPermission(ctx, "create-space", spaceRef(spaceid))
}

// SetSpaceQuota returns true when the user is allowed to change the spaces quota
func (p Permissions) SetSpaceQuota(ctx context.Context, spaceid string) bool {
	return p.checkPermission(ctx, "set-space-quota", spaceRef(spaceid))
}

// ManageSpaceProperties returns true when the user is allowed to change space properties (name/subtitle)
func (p Permissions) ManageSpaceProperties(ctx context.Context, spaceid string) bool {
	return p.checkPermission(ctx, "manage-space-properties", spaceRef(spaceid))
}

// SpaceAbility returns true when the user is allowed to enable/disable the space
func (p Permissions) SpaceAbility(ctx context.Context, spaceid string) bool {
	return p.checkPermission(ctx, "space-ability", spaceRef(spaceid))
}

// ListAllSpaces returns true when the user is allowed to list all spaces
func (p Permissions) ListAllSpaces(ctx context.Context) bool {
	return p.checkPermission(ctx, "list-all-spaces", nil)
}

// DeleteAllSpaces returns true when the user is allowed to delete all spaces
func (p Permissions) DeleteAllSpaces(ctx context.Context) bool {
	return p.checkPermission(ctx, "delete-all-spaces", nil)
}

// DeleteAllHomeSpaces returns true when the user is allowed to delete all home spaces
func (p Permissions) DeleteAllHomeSpaces(ctx context.Context) bool {
	return p.checkPermission(ctx, "delete-all-home-spaces", nil)
}

// checkRole returns true if the user has the given role on the space the node belongs to
func (p Permissions) checkRole(ctx context.Context, n *node.Node, role string) bool {
	rp, err := p.AssemblePermissions(ctx, n)
	if err != nil {
		return false
	}

	switch role {
	case "manager":
		// current workaround: check if RemoveGrant Permission exists
		return rp.RemoveGrant
	case "editor":
		// current workaround: check if InitiateFileUpload Permission exists
		return rp.InitiateFileUpload
	case "viewer":
		// current workaround: check if Stat Permission exists
		return rp.Stat
	default:
		return false
	}
}

// checkPermission is used to check a users space permissions
func (p Permissions) checkPermission(ctx context.Context, perm string, ref *provider.Reference) bool {
	user := ctxpkg.ContextMustGetUser(ctx)
	checkRes, err := p.space.CheckPermission(ctx, &cs3permissions.CheckPermissionRequest{
		Permission: perm,
		SubjectRef: &cs3permissions.SubjectReference{
			Spec: &cs3permissions.SubjectReference_UserId{
				UserId: user.Id,
			},
		},
		Ref: ref,
	})
	if err != nil {
		return false
	}

	return checkRes.Status.Code == v1beta11.Code_CODE_OK
}

func spaceRef(spaceid string) *provider.Reference {
	return &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: spaceid,
			// OpaqueId is the same, no need to transfer it
		},
	}
}
