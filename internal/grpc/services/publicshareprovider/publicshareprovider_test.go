package publicshareprovider_test

import (
	"context"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencloud-eu/reva/v2/internal/grpc/services/publicshareprovider"
	ctxpkg "github.com/opencloud-eu/reva/v2/pkg/ctx"
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/opencloud-eu/reva/v2/pkg/permission"
	"github.com/opencloud-eu/reva/v2/pkg/publicshare"
	"github.com/opencloud-eu/reva/v2/pkg/publicshare/manager/registry"
	"github.com/opencloud-eu/reva/v2/pkg/publicshare/mocks"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/status"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/opencloud-eu/reva/v2/pkg/utils"
	cs3mocks "github.com/opencloud-eu/reva/v2/tests/cs3mocks/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

func createPublicShareProvider(revaConfig map[string]interface{}, gs pool.Selectable[gateway.GatewayAPIClient], sm publicshare.Manager) (link.LinkAPIServer, error) {
	config, err := publicshareprovider.ParseConfig(revaConfig)
	if err != nil {
		return nil, err
	}
	passwordPolicy, err := publicshareprovider.ParsePasswordPolicy(config.PasswordPolicy)
	if err != nil {
		return nil, err
	}
	publicshareproviderService, err := publicshareprovider.New(gs, sm, config, passwordPolicy)
	if err != nil {
		return nil, err
	}
	return publicshareproviderService.(link.LinkAPIServer), nil
}

var _ = Describe("PublicShareProvider", func() {
	// declare in container nodes
	var (
		ctx                     context.Context
		checkPermissionResponse *permissions.CheckPermissionResponse
		statResourceResponse    *providerpb.StatResponse
		provider                link.LinkAPIServer
		manager                 *mocks.Manager
		gatewayClient           *cs3mocks.GatewayAPIClient
		gatewaySelector         pool.Selectable[gateway.GatewayAPIClient]
		resourcePermissions     *providerpb.ResourcePermissions
		linkPermissions         *providerpb.ResourcePermissions
		createdLink             *link.PublicShare
		revaConfig              map[string]interface{}
		user                    *userpb.User
	)

	BeforeEach(func() {
		// initialize in setup nodes
		manager = mocks.NewManager(GinkgoT())

		registry.Register("mockManager", func(m map[string]interface{}) (publicshare.Manager, error) {
			return manager, nil
		})

		pool.RemoveSelector("GatewaySelector" + "any")
		gatewayClient = cs3mocks.NewGatewayAPIClient(GinkgoT())
		gatewaySelector = pool.GetSelector[gateway.GatewayAPIClient](
			"GatewaySelector",
			"any",
			func(cc grpc.ClientConnInterface) gateway.GatewayAPIClient {
				return gatewayClient
			},
		)

		checkPermissionResponse = &permissions.CheckPermissionResponse{
			Status: status.NewOK(ctx),
		}

		user = &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
			Username: "alice",
		}
		ctx = ctxpkg.ContextSetUser(context.Background(), user)

		resourcePermissions = &providerpb.ResourcePermissions{
			// all permissions
			AddGrant:             true,
			CreateContainer:      true,
			Delete:               true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListGrants:           true,
			ListRecycle:          true,
			Move:                 true,
			PurgeRecycle:         true,
			RemoveGrant:          true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
			Stat:                 true,
			UpdateGrant:          true,
			DenyGrant:            true,
		}

		statResourceResponse = &providerpb.StatResponse{
			Status: status.NewOK(ctx),
			Info: &providerpb.ResourceInfo{
				Type:          providerpb.ResourceType_RESOURCE_TYPE_FILE,
				Path:          "./file.txt",
				PermissionSet: resourcePermissions,
			},
		}

		linkPermissions = &providerpb.ResourcePermissions{
			Stat:                 true,
			CreateContainer:      true,
			Delete:               true,
			GetPath:              true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
		}

		revaConfig = map[string]interface{}{
			"driver": "mockManager",
			"drivers": map[string]map[string]interface{}{
				"jsoncs3": {
					"provider_addr":                 "eu.opencloud.api.storage-system",
					"service_user_idp":              "internal",
					"enable_expired_shares_cleanup": true,
					"gateway_addr":                  "https://localhost:9200",
				},
			},
			"gateway_addr":                       "https://localhost:9200",
			"allowed_paths_for_shares":           []string{"/NewFolder"},
			"writeable_share_must_have_password": false,
			"public_share_must_have_password":    true,
			"password_policy": map[string]interface{}{
				"min_digits":               1,
				"min_characters":           8,
				"min_lowercase_characters": 1,
				"min_uppercase_characters": 1,
				"min_special_characters":   1,
				"banned_passwords_list": map[string]struct{}{
					"SecretPassword1!": {},
				},
			},
		}
		var err error
		provider, err = createPublicShareProvider(revaConfig, gatewaySelector, manager)
		Expect(err).ToNot(HaveOccurred())
		Expect(provider).ToNot(BeNil())
	})
	Describe("Creating a PublicShare", func() {
		BeforeEach(func() {
			gatewayClient.
				EXPECT().
				CheckPermission(
					mock.Anything,
					mock.Anything,
				).
				Return(checkPermissionResponse, nil)

			gatewayClient.
				EXPECT().
				Stat(mock.Anything, mock.Anything).
				Return(statResourceResponse, nil)

			createdLink = &link.PublicShare{
				PasswordProtected: true,
				Permissions: &link.PublicSharePermissions{
					Permissions: linkPermissions,
				},
				Creator: user.Id,
			}
		})
		It("creates a public share with password", func() {
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Password: "SecretPassw0rd!",
				},
				Description: "test",
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("has no user permission to create public share", func() {
			gatewayClient.
				EXPECT().
				CheckPermission(mock.Anything, mock.Anything).
				Unset()
			checkPermissionResponse = &permissions.CheckPermissionResponse{
				Status: status.NewPermissionDenied(ctx, nil, "permission denied"),
			}
			gatewayClient.
				EXPECT().
				CheckPermission(mock.Anything, mock.Anything).
				Return(checkPermissionResponse, nil)

			req := &link.CreatePublicShareRequest{}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))
			Expect(res.GetStatus().GetMessage()).To(Equal("no permission to create public links"))
		})
		It("has permission to create internal link", func() {
			// internal links are created with empty permissions
			linkPermissions := &providerpb.ResourcePermissions{}
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)
			// internal link creation should not check user permissions
			gatewayClient.EXPECT().CheckPermission(mock.Anything, mock.Anything).Unset()

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
				Description: "test",
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("has no share permission on the resource, can create internal link", func() {
			// internal links are created with empty permissions
			linkPermissions := &providerpb.ResourcePermissions{}
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			// internal link creation should not check user permissions
			gatewayClient.EXPECT().CheckPermission(mock.Anything, mock.Anything).Unset()

			// downgrade user permissions on resource to have no share permission
			resourcePermissions.AddGrant = false

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
				Description: "test",
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("fails to check create public share user permission", func() {
			gatewayClient.EXPECT().CheckPermission(mock.Anything, mock.Anything).Unset()
			gatewayClient.
				EXPECT().
				CheckPermission(mock.Anything, mock.Anything).
				Return(nil, errors.New("transport error"))

			req := &link.CreatePublicShareRequest{}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
			Expect(res.GetStatus().GetMessage()).To(Equal("failed check user permission to write public link"))
		})
		It("has no share permission on the resource", func() {
			req := &link.CreatePublicShareRequest{}

			// downgrade user permissions to have no share permission
			resourcePermissions.AddGrant = false

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("no share permission"))
		})
		It("tries to share with higher permissions than granted on the resource", func() {
			// set resource permissions lower than the requested share permissions
			resourcePermissions.Delete = false

			req := &link.CreatePublicShareRequest{
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("insufficient permissions to create that kind of share"))
		})
		It("tries to share inside a path which is not allowed", func() {
			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_FAILED_PRECONDITION))
			Expect(res.GetStatus().GetMessage()).To(Equal("share creation is not allowed for the specified path"))
		})
		It("tries to share personal space root", func() {
			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
					Id: &providerpb.ResourceId{
						StorageId: "storage-id",
						OpaqueId:  "admin-id",
						SpaceId:   "admin-id",
					},
					Space: &providerpb.StorageSpace{SpaceType: "personal"},
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("cannot create link on personal space root"))
		})
		It("tries to share a project space root", func() {
			manager.EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
					Id: &providerpb.ResourceId{
						StorageId: "storage-id",
						OpaqueId:  "project-id",
						SpaceId:   "project-id",
					},
					Space: &providerpb.StorageSpace{SpaceType: "project"},
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Password: "SecretPassw0rd!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("creates a new quicklink", func() {
			createdLink.Quicklink = true
			manager.EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			// no existing quicklinks
			var links = []*link.PublicShare{}
			manager.EXPECT().ListPublicShares(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(links, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					ArbitraryMetadata: &providerpb.ArbitraryMetadata{Metadata: map[string]string{"quicklink": "true"}},
					Path:              "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Password: "SecretPassw0rd!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("creates a quicklink which already exists", func() {
			// create a quicklink with viewer permissions
			// there is already a quicklink present for that resource
			// instead of creating a new quicklink, we return the existing quicklink without updating it
			existingLink := &link.PublicShare{
				PasswordProtected: true,
				Permissions: &link.PublicSharePermissions{
					Permissions: linkPermissions,
				},
				Quicklink:   true,
				Description: "Quicklink",
			}
			var links = []*link.PublicShare{}
			links = append(links, existingLink)

			// confirm that list public shares is called with the correct filters
			manager.
				EXPECT().
				ListPublicShares(
					mock.Anything,
					mock.Anything,
					mock.MatchedBy(func(filters []*link.ListPublicSharesRequest_Filter) bool {
						return filters[0].Type == link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID &&
							filters[0].GetResourceId().GetOpaqueId() == "project-id" &&
							filters[0].GetResourceId().GetStorageId() == "storage-id" &&
							filters[0].GetResourceId().GetSpaceId() == "project-id"
					}),
					mock.Anything).
				Return(links, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					// this should create a quicklink
					ArbitraryMetadata: &providerpb.ArbitraryMetadata{Metadata: map[string]string{"quicklink": "true"}},
					Path:              "./NewFolder/file.txt",
					Id: &providerpb.ResourceId{
						StorageId: "storage-id",
						OpaqueId:  "project-id",
						SpaceId:   "project-id",
					},
					Space: &providerpb.StorageSpace{SpaceType: "project"},
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: &providerpb.ResourcePermissions{
							Stat:                 true,
							InitiateFileDownload: true,
							ListContainer:        true,
						},
					},
					Password: "SecretPassw0rd!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(existingLink))
		})
		It("create public share with expiration date in the past", func() {
			yesterday := time.Now().AddDate(0, 0, -1)
			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Expiration: utils.TimeToTS(yesterday),
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(ContainSubstring("expiration date is in the past"))
		})
		It("create public share with valid expiration date", func() {
			tomorrow := time.Now().AddDate(0, 0, 1)
			tsTomorrow := utils.TimeToTS(tomorrow)

			createdLink.Expiration = tsTomorrow
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Expiration: tsTomorrow,
					Password:   "SecretPassw0rd!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("creates a public share without a password", func() {
			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("password protection is enforced"))
		})
		It("creates a viewable public share without a password", func() {
			// set password enforcement only on writeable shares
			revaConfig["public_share_must_have_password"] = false
			revaConfig["writeable_share_must_have_password"] = true

			provider, err := createPublicShareProvider(revaConfig, gatewaySelector, manager)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider).ToNot(BeNil())

			linkPermissions = &providerpb.ResourcePermissions{
				Stat:                 true,
				ListContainer:        true,
				InitiateFileDownload: true,
			}
			createdLink.PasswordProtected = false
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("creates an editable public share without a password", func() {
			// set password enforcement to false on all public shares
			revaConfig["public_share_must_have_password"] = false
			revaConfig["writeable_share_must_have_password"] = false

			provider, err := createPublicShareProvider(revaConfig, gatewaySelector, manager)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider).ToNot(BeNil())

			linkPermissions = &providerpb.ResourcePermissions{
				Stat:                 true,
				ListContainer:        true,
				InitiateFileDownload: true,
				InitiateFileUpload:   true,
				CreateContainer:      true,
				Delete:               true,
				Move:                 true,
			}
			createdLink.PasswordProtected = false
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(createdLink, nil)

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
			Expect(res.GetShare()).To(Equal(createdLink))
		})
		It("applies the password policy even if no enforcement is configured", func() {
			// set password enforcement to false on all public shares
			revaConfig["public_share_must_have_password"] = false
			revaConfig["writeable_share_must_have_password"] = false

			provider, err := createPublicShareProvider(revaConfig, gatewaySelector, manager)
			Expect(err).ToNot(HaveOccurred())
			Expect(provider).ToNot(BeNil())

			linkPermissions = &providerpb.ResourcePermissions{
				Stat:                 true,
				ListContainer:        true,
				InitiateFileDownload: true,
				InitiateFileUpload:   true,
				CreateContainer:      true,
				Delete:               true,
				Move:                 true,
			}

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Password: "SecretPassword1!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("unfortunately, your password is commonly used. please pick a harder-to-guess password for your safety"))
		})
		It("returns internal server error when share manager not functional", func() {
			manager.
				EXPECT().
				CreatePublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, errors.New("transport error"))

			req := &link.CreatePublicShareRequest{
				ResourceInfo: &providerpb.ResourceInfo{
					Owner: &userpb.UserId{
						OpaqueId: "alice",
					},
					Path: "./NewFolder/file.txt",
				},
				Grant: &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					Password: "SecretPassw0rd123!",
				},
			}

			res, err := provider.CreatePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
			Expect(res.GetStatus().GetMessage()).To(Equal("error persisting public share:transport error"))
		})
	})
	Describe("Removing a PublicShare", func() {
		BeforeEach(func() {
			createdLink.Id = &link.PublicShareId{
				OpaqueId: "share-id",
			}
		})
		It("cannot load existing share", func() {
			manager.
				EXPECT().
				GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, errors.New("transport error"))

			req := &link.RemovePublicShareRequest{
				Ref: &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: "share-id",
						},
					},
				},
			}
			res, err := provider.RemovePublicShare(ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
			Expect(res.GetStatus().GetMessage()).To(Equal("error loading public share"))
		})
		It("can remove an existing share as a creator", func() {
			manager.
				EXPECT().
				GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Once().
				Return(
					createdLink, nil)

			gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything).Return(statResourceResponse, nil)

			manager.
				EXPECT().
				RevokePublicShare(
					mock.Anything,
					mock.MatchedBy(func(callingUser *userpb.User) bool {
						return callingUser == user
					}),
					mock.MatchedBy(func(linkRef *link.PublicShareReference) bool {
						return linkRef.GetId().GetOpaqueId() == "share-id"
					})).
				Once().
				Return(nil)

			req := &link.RemovePublicShareRequest{
				Ref: &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: "share-id",
						},
					},
				},
			}
			res, err := provider.RemovePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
		})
		It("can remove an existing share as a space manager", func() {
			// link is neither owned nor created by the acting user
			createdLink.Creator = &userpb.UserId{
				OpaqueId: "admin",
			}
			manager.
				EXPECT().
				GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Once().Return(createdLink, nil)

			gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything).Return(statResourceResponse, nil)

			manager.
				EXPECT().
				RevokePublicShare(
					mock.Anything,
					mock.MatchedBy(
						func(callingUser *userpb.User) bool {
							return callingUser == user
						}),
					mock.MatchedBy(
						func(linkRef *link.PublicShareReference) bool {
							return linkRef.GetId().GetOpaqueId() == "share-id"
						})).
				Once().Return(nil)

			req := &link.RemovePublicShareRequest{
				Ref: &link.PublicShareReference{
					Spec: &link.PublicShareReference_Id{
						Id: &link.PublicShareId{
							OpaqueId: "share-id",
						},
					},
				},
			}
			res, err := provider.RemovePublicShare(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
		})
		Context("when the user is not the owner or creator", func() {
			BeforeEach(func() {
				// link is neither owned nor created by the acting user
				createdLink.Creator = &userpb.UserId{
					OpaqueId: "admin",
				}

				// stat the shared resource to get the users resource permissions
				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Return(statResourceResponse, nil)
			})
			It("cannot remove an existing share as a space editor", func() {
				// downgrade user permissions to editor
				resourcePermissions.AddGrant = false
				resourcePermissions.UpdateGrant = false
				resourcePermissions.RemoveGrant = false

				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(createdLink, nil)

				req := &link.RemovePublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}
				res, err := provider.RemovePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))
				Expect(res.GetStatus().GetMessage()).To(Equal("no permission to delete public share"))
			})
			It("triggers an internal server error when the share manager is not available", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(createdLink, nil)

				// Share manager is not functional
				By("experiencing a share delete error in the persistence layer")
				manager.EXPECT().RevokePublicShare(mock.Anything, mock.Anything, mock.Anything).
					Once().Return(errors.New("delete error"))

				req := &link.RemovePublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}
				res, err := provider.RemovePublicShare(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(Equal("error deleting public share"))
			})
			It("triggers an internal server error when the storage provider is not functional", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(createdLink, nil)

				// Storage Provider is not functional
				By("experiencing a transport error when trying to stat the shared resource")
				gatewayClient.EXPECT().
					Stat(mock.Anything, mock.Anything).Unset()
				gatewayClient.EXPECT().
					Stat(mock.Anything, mock.Anything).
					Return(nil, errors.New("transport error"))

				req := &link.RemovePublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}
				res, err := provider.RemovePublicShare(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(Equal("failed to stat shared resource"))
			})
		})
	})
	Describe("Updating a PublicShare", func() {
		var (
			existingLink *link.PublicShare
			updatedLink  *link.PublicShare
		)

		BeforeEach(func() {
			// stat the shared resource to get the users resource permissions
			gatewayClient.
				EXPECT().
				Stat(mock.Anything, mock.Anything).
				Return(statResourceResponse, nil)

			existingLink = &link.PublicShare{
				Id: &link.PublicShareId{
					OpaqueId: "share-id",
				},
				Token: "token",
				Permissions: &link.PublicSharePermissions{
					Permissions: &providerpb.ResourcePermissions{
						Stat:                 true,
						ListContainer:        true,
						InitiateFileDownload: true,
						AddGrant:             true,
					},
				},
				Creator: user.GetId(),
			}
		})
		Context("succeeds when the user downgrades a public link to internal", func() {
			BeforeEach(func() {
				linkPermissions = &providerpb.ResourcePermissions{}
				updatedLink = &link.PublicShare{
					Id: &link.PublicShareId{
						OpaqueId: "share-id",
					},
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					DisplayName: "Updated Link",
				}
			})
			It("fails when it cannot load the existing share", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("transport error"))

				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
			})
			It("fails when it cannot connect to the storage provider", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()
				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Return(nil, errors.New("transport error"))

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
			})
			It("fails when it cannot find the shared resource", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()

				statResourceResponse.Status = status.NewNotFound(context.TODO(), "not found")
				statResourceResponse.Info = nil
				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Return(statResourceResponse, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_NOT_FOUND))
			})
			It("fails when it cannot store share information", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				manager.
					EXPECT().
					UpdatePublicShare(mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errors.New("storage error"))

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(Equal("storage error"))
			})
			It("fails when the user is neither the creator nor the owner of the share", func() {
				existingLink.Creator = &userpb.UserId{
					OpaqueId: "admin",
				}
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				checkPermissionResponse.Status.Code = rpc.Code_CODE_PERMISSION_DENIED
				gatewayClient.
					EXPECT().
					CheckPermission(mock.Anything, mock.Anything).
					Return(checkPermissionResponse, nil)

				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},

					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))

			})
			It("fails when user is neither the resource owner nor the share creator", func() {
				existingLink.Creator = &userpb.UserId{
					OpaqueId: "admin",
				}
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				checkPermissionResponse.Status.Code = rpc.Code_CODE_PERMISSION_DENIED
				gatewayClient.
					EXPECT().
					CheckPermission(mock.Anything, mock.Anything).
					Return(checkPermissionResponse, nil)

				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()

				linkPermissions.Delete = true
				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},

					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))

			})
			It("succeeds", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				manager.
					EXPECT().
					UpdatePublicShare(mock.Anything, mock.Anything, mock.Anything).
					Once().Return(updatedLink, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},

					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
				Expect(res.GetShare()).To(Equal(updatedLink))
			})
			It("succeeds even if the user is no manager or owner on the resource", func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				manager.
					EXPECT().
					UpdatePublicShare(mock.Anything, mock.Anything, mock.Anything).
					Once().Return(updatedLink, nil)

				statResourceResponse.Info.PermissionSet.UpdateGrant = false

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},

					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
				Expect(res.GetShare()).To(Equal(updatedLink))
			})
		})
		Context("when the user changes permissions", func() {
			BeforeEach(func() {
				linkPermissions = &providerpb.ResourcePermissions{
					Stat:                 true,
					AddGrant:             true,
					Delete:               true,
					Move:                 true,
					InitiateFileDownload: true,
					InitiateFileUpload:   true,
					CreateContainer:      true,
					ListContainer:        true,
					ListGrants:           true,
				}

				updatedLink = &link.PublicShare{
					Id: &link.PublicShareId{
						OpaqueId: "share-id",
					},
					Permissions: &link.PublicSharePermissions{
						Permissions: linkPermissions,
					},
					DisplayName: "Updated Link",
				}

				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)
			})
			It("fails when the user has not enough permissions on the resource", func() {
				statResourceResponse.Info.PermissionSet.Delete = false

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
				Expect(res.GetStatus().GetMessage()).To(Equal("insufficient permissions to update that kind of share"))
			})
			It("fails when the permissions client is not responding", func() {
				gatewayClient.
					EXPECT().
					CheckPermission(mock.Anything, mock.Anything).
					Return(nil, errors.New("transport error"))

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(Equal("transport error"))
			})
			It("fails when the user has no permission to write public shares and is not the creator", func() {
				checkPermissionResponse.Status.Code = rpc.Code_CODE_PERMISSION_DENIED
				gatewayClient.
					EXPECT().
					CheckPermission(mock.Anything, mock.Anything).
					Return(checkPermissionResponse, nil)

				existingLink.Creator = &userpb.UserId{
					OpaqueId: "admin",
				}
				gatewayClient.
					EXPECT().
					Stat(mock.Anything, mock.Anything).
					Unset()

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))
				Expect(res.GetStatus().GetMessage()).To(Equal("no permission to update public share"))
			})
			It("fails when the user is not the creator and has no manager or owner permissions", func() {
				existingLink.Creator = &userpb.UserId{
					OpaqueId: "admin",
				}

				gatewayClient.
					EXPECT().
					CheckPermission(mock.Anything, mock.Anything).
					Return(checkPermissionResponse, nil)

				statResourceResponse.Info.PermissionSet.UpdateGrant = false

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: linkPermissions,
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))
				Expect(res.GetStatus().GetMessage()).To(Equal("no permission to update public share"))
			})
			It("fails when the expiration date is in the past", func() {
				yesterday := time.Now().AddDate(0, 0, -1)
				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
						Grant: &link.Grant{
							Expiration: utils.TimeToTS(yesterday),
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("expiration date is in the past"))
			})
			It("fails when the password is changed to on a writable share", func() {
				gatewayClient.
					EXPECT().
					CheckPermission(
						mock.Anything,
						mock.MatchedBy(
							func(req *permissions.CheckPermissionRequest) bool {
								return req.GetPermission() == permission.DeleteReadOnlyPassword
							},
						),
					).
					Return(checkPermissionResponse, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
						Grant: &link.Grant{
							Password: "",
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("password protection is enforced"))
			})
			It("fails when the password is already empty on a writable share", func() {
				gatewayClient.
					EXPECT().
					CheckPermission(
						mock.Anything,
						mock.MatchedBy(
							func(req *permissions.CheckPermissionRequest) bool {
								return req.GetPermission() == permission.DeleteReadOnlyPassword
							},
						),
					).
					Return(checkPermissionResponse, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: &providerpb.ResourcePermissions{
									Stat:                 true,
									ListContainer:        true,
									InitiateFileDownload: true,
									Move:                 true,
								},
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("password protection is enforced"))
			})
			It("succeeds when the password is empty on a readable share with opt out permission", func() {
				gatewayClient.
					EXPECT().
					CheckPermission(
						mock.Anything,
						mock.MatchedBy(
							func(req *permissions.CheckPermissionRequest) bool {
								return req.GetPermission() == permission.DeleteReadOnlyPassword
							},
						),
					).
					Return(checkPermissionResponse, nil)

				manager.
					EXPECT().
					UpdatePublicShare(mock.Anything, mock.Anything, mock.Anything).
					Once().Return(updatedLink, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
						Grant: &link.Grant{
							Permissions: &link.PublicSharePermissions{
								Permissions: &providerpb.ResourcePermissions{
									Stat:                 true,
									ListContainer:        true,
									InitiateFileDownload: true,
								},
							},
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_OK))
				Expect(res.GetShare()).To(Equal(updatedLink))
			})
			It("fails when the updated password doesn't fulfil the policy", func() {
				gatewayClient.
					EXPECT().
					CheckPermission(
						mock.Anything,
						mock.MatchedBy(
							func(req *permissions.CheckPermissionRequest) bool {
								return req.GetPermission() == permission.DeleteReadOnlyPassword
							},
						),
					).
					Return(checkPermissionResponse, nil)

				req := &link.UpdatePublicShareRequest{
					Update: &link.UpdatePublicShareRequest_Update{
						Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
						Grant: &link.Grant{
							Password: "Test",
						},
					},
				}

				res, err := provider.UpdatePublicShare(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("characters are required"))
			})
		})
		Context("when the user gets a public share", func() {
			BeforeEach(func() {
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything).Unset()
			})
			It("succeeds", func() {
				res, err := provider.GetPublicShare(ctx, &link.GetPublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetShare()).To(Equal(existingLink))
			})
			It("fails when it finds no share", func() {
				manager.EXPECT().GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errtypes.NotFound("not found"))
				res, err := provider.GetPublicShare(ctx, &link.GetPublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_NOT_FOUND))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("not found"))
			})
			It("fails when it finds no share due to internal error", func() {
				manager.EXPECT().GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errtypes.InternalError("internal error"))
				res, err := provider.GetPublicShare(ctx, &link.GetPublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("internal error"))
			})
			It("fails when the share manager response is empty", func() {
				manager.EXPECT().GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShare(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, nil)
				res, err := provider.GetPublicShare(ctx, &link.GetPublicShareRequest{
					Ref: &link.PublicShareReference{
						Spec: &link.PublicShareReference_Id{
							Id: &link.PublicShareId{
								OpaqueId: "share-id",
							},
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_NOT_FOUND))
				Expect(res.GetStatus().GetMessage()).To(ContainSubstring("not found"))
			})
		})
		Context("when the user gets a public share by token", func() {
			BeforeEach(func() {
				manager.
					EXPECT().
					GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(existingLink, nil)

				gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything).Unset()
			})
			It("succeeds", func() {
				res, err := provider.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
					Token: "token",
					Authentication: &link.PublicShareAuthentication{
						Spec: &link.PublicShareAuthentication_Password{
							Password: "password",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetShare()).To(Equal(existingLink))
			})
			It("fails with invalid credentials", func() {
				manager.EXPECT().GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errtypes.InvalidCredentials("wrong password"))

				res, err := provider.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
					Token: "token",
					Authentication: &link.PublicShareAuthentication{
						Spec: &link.PublicShareAuthentication_Password{
							Password: "password",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_PERMISSION_DENIED))
				Expect(res.GetStatus().GetMessage()).To(Equal("wrong password"))
			})
			It("fails with an unknown token", func() {
				manager.EXPECT().GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errtypes.NotFound("public share not found"))

				res, err := provider.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
					Token: "token",
					Authentication: &link.PublicShareAuthentication{
						Spec: &link.PublicShareAuthentication_Password{
							Password: "password",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_NOT_FOUND))
				Expect(res.GetStatus().GetMessage()).To(Equal("unknown token"))
			})
			It("fails with an unknown error", func() {
				manager.EXPECT().GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()
				manager.
					EXPECT().
					GetPublicShareByToken(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Once().Return(nil, errtypes.InternalError("internal error"))

				res, err := provider.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
					Token: "token",
					Authentication: &link.PublicShareAuthentication{
						Spec: &link.PublicShareAuthentication_Password{
							Password: "password",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INTERNAL))
				Expect(res.GetStatus().GetMessage()).To(Equal("unexpected error"))
			})
		})
	})
})
