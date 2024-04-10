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
	"github.com/cs3org/reva/v2/internal/grpc/services/publicshareprovider"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/v2/pkg/publicshare/mocks"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/utils"
	cs3mocks "github.com/cs3org/reva/v2/tests/cs3mocks/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

		createdLink = &link.PublicShare{
			PasswordProtected: true,
			Permissions: &link.PublicSharePermissions{
				Permissions: linkPermissions,
			},
			Creator: user.Id,
		}

		revaConfig = map[string]interface{}{
			"driver": "mockManager",
			"drivers": map[string]map[string]interface{}{
				"jsoncs3": {
					"provider_addr":                 "com.owncloud.api.storage-system",
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
		It("has no share permission on the resource to create internal link", func() {
			// internal links are created with empty permissions
			linkPermissions := &providerpb.ResourcePermissions{}

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
			Expect(res.GetStatus().GetCode()).To(Equal(rpc.Code_CODE_INVALID_ARGUMENT))
			Expect(res.GetStatus().GetMessage()).To(Equal("no share permission"))
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
		It("create public share with valid expiration", func() {
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
})
