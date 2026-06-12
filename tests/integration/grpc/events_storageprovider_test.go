// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package grpc_test

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storagep "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/owncloud/reva/v2/pkg/auth/scope"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	revaevents "github.com/owncloud/reva/v2/pkg/events"
	"github.com/owncloud/reva/v2/pkg/events/stream"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
	jwt "github.com/owncloud/reva/v2/pkg/token/manager/jwt"
)

func consumeEvent(ch <-chan revaevents.Event, timeout time.Duration) (revaevents.Event, error) {
	select {
	case ev := <-ch:
		return ev, nil
	case <-time.After(timeout):
		return revaevents.Event{}, fmt.Errorf("timeout waiting for event after %s", timeout)
	}
}

func mustDrainEvent(ch <-chan revaevents.Event) {
	_, err := consumeEvent(ch, 3*time.Second)
	Expect(err).ToNot(HaveOccurred())
}

const (
	// spaceID and ownerID are intentionally equal: in decomposedfs a personal
	// space is bootstrapped with a root node whose ID equals the owner's user ID.
	spaceID = "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"
	ownerID = "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"
)

var _ = Describe("ocis storage provider events", func() {
	var (
		revads         map[string]*Revad
		ctx            context.Context
		providerClient storagep.ProviderAPIClient
		spacesClient   storagep.SpacesAPIClient
		eventCh        <-chan revaevents.Event
		stopNats       func()

		user = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "0.0.0.0:19000",
				OpaqueId: ownerID,
				Type:     userpb.UserType_USER_TYPE_PRIMARY,
			},
			Username: "einstein",
		}
	)

	rootRef := func() *storagep.Reference {
		return &storagep.Reference{
			ResourceId: &storagep.ResourceId{SpaceId: spaceID, OpaqueId: spaceID},
		}
	}

	pathRef := func(path string) *storagep.Reference {
		return &storagep.Reference{
			ResourceId: &storagep.ResourceId{SpaceId: spaceID, OpaqueId: spaceID},
			Path:       path,
		}
	}

	JustBeforeEach(func() {
		var err error

		natsAddr, stop, err := startNats()
		Expect(err).ToNot(HaveOccurred())
		stopNats = stop

		revads, err = startRevads([]RevadConfig{
			{Name: "storage", Config: "storageprovider-ocis-events.toml"},
			{Name: "permissions", Config: "permissions-ocis-ci.toml"},
		}, map[string]string{
			"nats_address": natsAddr,
			"enable_home":  "true",
		})
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		tokenManager, err := jwt.New(map[string]interface{}{"secret": "changemeplease"})
		Expect(err).ToNot(HaveOccurred())
		sc, err := scope.AddOwnerScope(nil)
		Expect(err).ToNot(HaveOccurred())
		t, err := tokenManager.MintToken(ctx, user, sc)
		Expect(err).ToNot(HaveOccurred())
		ctx = ctxpkg.ContextSetToken(ctx, t)
		ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, t)
		ctx = ctxpkg.ContextSetUser(ctx, user)

		providerClient, err = pool.GetStorageProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())
		spacesClient, err = pool.GetSpacesProviderServiceClient(revads["storage"].GrpcAddress)
		Expect(err).ToNot(HaveOccurred())

		natsStream, err := stream.NatsFromConfig("events-test", true, stream.NatsConfig{
			Endpoint: natsAddr,
		})
		Expect(err).ToNot(HaveOccurred())
		eventCh, err = revaevents.Consume(natsStream, "integration-test",
			revaevents.ContainerCreated{},
			revaevents.FileTouched{},
			revaevents.FileLocked{},
			revaevents.FileUnlocked{},
			revaevents.FileDownloaded{},
			revaevents.ItemMoved{},
			revaevents.ItemTrashed{},
			revaevents.ItemRestored{},
			revaevents.ItemPurged{},
			revaevents.SpaceCreated{},
			revaevents.SpaceRenamed{},
			revaevents.SpaceUpdated{},
			revaevents.SpaceEnabled{},
			revaevents.SpaceDisabled{},
			revaevents.SpaceDeleted{},
			revaevents.SpaceShared{},
			revaevents.SpaceShareUpdated{},
			revaevents.SpaceUnshared{},
		)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentSpecReport().Failed())).To(Succeed())
		}
		if stopNats != nil {
			stopNats()
		}
	})

	// SpaceCreated
	It("emits SpaceCreated with correct fields", func() {
		res, err := spacesClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
			Owner: user,
			Type:  "personal",
			Name:  user.Id.OpaqueId,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

		ev, err := consumeEvent(eventCh, 3*time.Second)
		Expect(err).ToNot(HaveOccurred())

		typed, ok := ev.Event.(revaevents.SpaceCreated)
		Expect(ok).To(BeTrue(), "expected SpaceCreated, got %T", ev.Event)
		Expect(typed.Executant).ToNot(BeNil())
		Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
		Expect(typed.ID).ToNot(BeNil())
		Expect(typed.Owner).ToNot(BeNil())
	})

	Context("with a personal space", func() {
		var createdSpaceID *storagep.StorageSpaceId

		JustBeforeEach(func() {
			res, err := spacesClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
				Owner: user,
				Type:  "personal",
				Name:  user.Id.OpaqueId,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
			createdSpaceID = res.StorageSpace.Id
			mustDrainEvent(eventCh)
		})

		// ContainerCreated
		It("emits ContainerCreated with correct fields", func() {
			res, err := providerClient.CreateContainer(ctx, &storagep.CreateContainerRequest{Ref: pathRef("/newdir")})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			ev, err := consumeEvent(eventCh, 3*time.Second)
			Expect(err).ToNot(HaveOccurred())

			typed, ok := ev.Event.(revaevents.ContainerCreated)
			Expect(ok).To(BeTrue(), "expected ContainerCreated, got %T", ev.Event)
			Expect(typed.Executant).ToNot(BeNil())
			Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
			Expect(typed.Ref).ToNot(BeNil())
			Expect(typed.SpaceOwner).ToNot(BeNil())
		})

		// FileTouched
		It("emits FileTouched with correct fields", func() {
			res, err := providerClient.TouchFile(ctx, &storagep.TouchFileRequest{Ref: pathRef("/touched.txt")})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			ev, err := consumeEvent(eventCh, 3*time.Second)
			Expect(err).ToNot(HaveOccurred())

			typed, ok := ev.Event.(revaevents.FileTouched)
			Expect(ok).To(BeTrue(), "expected FileTouched, got %T", ev.Event)
			Expect(typed.Executant).ToNot(BeNil())
			Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
			Expect(typed.Ref).ToNot(BeNil())
			Expect(typed.SpaceOwner).ToNot(BeNil())
		})

		Context("with an existing file", func() {
			JustBeforeEach(func() {
				res, err := providerClient.TouchFile(ctx, &storagep.TouchFileRequest{Ref: pathRef("/lockme.txt")})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				mustDrainEvent(eventCh)
			})

			// FileLocked
			It("emits FileLocked with correct fields", func() {
				res, err := providerClient.SetLock(ctx, &storagep.SetLockRequest{
					Ref:  pathRef("/lockme.txt"),
					Lock: &storagep.Lock{LockId: "test-lock-id", Type: storagep.LockType_LOCK_TYPE_WRITE},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.FileLocked)
				Expect(ok).To(BeTrue(), "expected FileLocked, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Ref).ToNot(BeNil())
			})

			// FileUnlocked
			It("emits FileUnlocked with correct fields", func() {
				_, err := providerClient.SetLock(ctx, &storagep.SetLockRequest{
					Ref:  pathRef("/lockme.txt"),
					Lock: &storagep.Lock{LockId: "test-lock-id", Type: storagep.LockType_LOCK_TYPE_WRITE},
				})
				Expect(err).ToNot(HaveOccurred())
				mustDrainEvent(eventCh)

				res, err := providerClient.Unlock(ctx, &storagep.UnlockRequest{
					Ref:  pathRef("/lockme.txt"),
					Lock: &storagep.Lock{LockId: "test-lock-id", Type: storagep.LockType_LOCK_TYPE_WRITE},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.FileUnlocked)
				Expect(ok).To(BeTrue(), "expected FileUnlocked, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Ref).ToNot(BeNil())
			})

			// FileDownloaded
			It("emits FileDownloaded with correct fields", func() {
				res, err := providerClient.InitiateFileDownload(ctx, &storagep.InitiateFileDownloadRequest{Ref: pathRef("/lockme.txt")})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.FileDownloaded)
				Expect(ok).To(BeTrue(), "expected FileDownloaded, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Ref).ToNot(BeNil())
			})

			// ItemMoved
			It("emits ItemMoved with correct fields", func() {
				res, err := providerClient.Move(ctx, &storagep.MoveRequest{
					Source:      pathRef("/lockme.txt"),
					Destination: pathRef("/moved.txt"),
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.ItemMoved)
				Expect(ok).To(BeTrue(), "expected ItemMoved, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Ref).ToNot(BeNil())
				Expect(typed.OldReference).ToNot(BeNil())
				Expect(typed.SpaceOwner).ToNot(BeNil())
			})

			// ItemTrashed
			It("emits ItemTrashed with correct fields", func() {
				res, err := providerClient.Delete(ctx, &storagep.DeleteRequest{Ref: pathRef("/lockme.txt")})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.ItemTrashed)
				Expect(ok).To(BeTrue(), "expected ItemTrashed, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Ref).ToNot(BeNil())
				Expect(typed.ID).ToNot(BeNil())
				Expect(typed.SpaceOwner).ToNot(BeNil())
			})
		})

		Context("with a trashed item", func() {
			var recycleKey string

			JustBeforeEach(func() {
				_, err := providerClient.TouchFile(ctx, &storagep.TouchFileRequest{Ref: pathRef("/tobedeleted.txt")})
				Expect(err).ToNot(HaveOccurred())
				mustDrainEvent(eventCh)

				_, err = providerClient.Delete(ctx, &storagep.DeleteRequest{Ref: pathRef("/tobedeleted.txt")})
				Expect(err).ToNot(HaveOccurred())

				trashedEv, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())
				typed, ok := trashedEv.Event.(revaevents.ItemTrashed)
				Expect(ok).To(BeTrue())
				recycleKey = typed.ID.GetOpaqueId()
				Expect(recycleKey).ToNot(BeEmpty())
			})

			// ItemRestored
			It("emits ItemRestored with correct fields", func() {
				res, err := providerClient.RestoreRecycleItem(ctx, &storagep.RestoreRecycleItemRequest{
					Ref: rootRef(),
					Key: recycleKey,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.ItemRestored)
				Expect(ok).To(BeTrue(), "expected ItemRestored, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.Key).ToNot(BeEmpty())
				Expect(typed.SpaceOwner).ToNot(BeNil())
			})

			// ItemPurged
			It("emits ItemPurged with correct fields", func() {
				res, err := providerClient.PurgeRecycle(ctx, &storagep.PurgeRecycleRequest{
					Ref: rootRef(),
					Key: recycleKey,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.ItemPurged)
				Expect(ok).To(BeTrue(), "expected ItemPurged, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
			})
		})

		// SpaceRenamed
		It("emits SpaceRenamed with correct fields", func() {
			res, err := spacesClient.UpdateStorageSpace(ctx, &storagep.UpdateStorageSpaceRequest{
				StorageSpace: &storagep.StorageSpace{
					Id:   createdSpaceID,
					Name: "renamedspace",
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			ev, err := consumeEvent(eventCh, 3*time.Second)
			Expect(err).ToNot(HaveOccurred())

			typed, ok := ev.Event.(revaevents.SpaceRenamed)
			Expect(ok).To(BeTrue(), "expected SpaceRenamed, got %T", ev.Event)
			Expect(typed.Executant).ToNot(BeNil())
			Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
			Expect(typed.Name).To(Equal("renamedspace"))
			Expect(typed.ID).ToNot(BeNil())
		})

		// SpaceUpdated — uses description because quota requires an admin-only permission
		// (Drives.ReadWritePersonalQuota) that the demo permissions driver denies.
		It("emits SpaceUpdated with correct fields", func() {
			res, err := spacesClient.UpdateStorageSpace(ctx, &storagep.UpdateStorageSpaceRequest{
				StorageSpace: &storagep.StorageSpace{
					Id: createdSpaceID,
					Opaque: &typespb.Opaque{
						Map: map[string]*typespb.OpaqueEntry{
							"description": {Decoder: "plain", Value: []byte("updated description")},
						},
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

			ev, err := consumeEvent(eventCh, 3*time.Second)
			Expect(err).ToNot(HaveOccurred())

			typed, ok := ev.Event.(revaevents.SpaceUpdated)
			Expect(ok).To(BeTrue(), "expected SpaceUpdated, got %T", ev.Event)
			Expect(typed.Executant).ToNot(BeNil())
			Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
			Expect(typed.ID).ToNot(BeNil())
		})

		// SpaceDisabled / SpaceEnabled / SpaceDeleted — use a project space because disabling a
		// personal space requires the DeleteAllHomeSpaces admin permission. The creator of a
		// project space is its manager and can disable/re-enable/purge it without admin rights.
		Context("with a project space", func() {
			var projID *storagep.StorageSpaceId

			JustBeforeEach(func() {
				projRes, err := spacesClient.CreateStorageSpace(ctx, &storagep.CreateStorageSpaceRequest{
					Owner: user,
					Type:  "project",
					Name:  "projectspace",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(projRes.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))
				projID = projRes.StorageSpace.Id
				mustDrainEvent(eventCh)
			})

			It("emits SpaceDisabled with correct fields", func() {
				res, err := spacesClient.DeleteStorageSpace(ctx, &storagep.DeleteStorageSpaceRequest{
					Id: projID,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.SpaceDisabled)
				Expect(ok).To(BeTrue(), "expected SpaceDisabled, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.ID).ToNot(BeNil())
			})

			Context("with a disabled project space", func() {
				JustBeforeEach(func() {
					_, err := spacesClient.DeleteStorageSpace(ctx, &storagep.DeleteStorageSpaceRequest{Id: projID})
					Expect(err).ToNot(HaveOccurred())
					mustDrainEvent(eventCh)
				})

				It("emits SpaceEnabled with correct fields", func() {
					res, err := spacesClient.UpdateStorageSpace(ctx, &storagep.UpdateStorageSpaceRequest{
						StorageSpace: &storagep.StorageSpace{Id: projID},
						Opaque: &typespb.Opaque{
							Map: map[string]*typespb.OpaqueEntry{
								"restore": {Decoder: "plain", Value: []byte("true")},
							},
						},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					ev, err := consumeEvent(eventCh, 3*time.Second)
					Expect(err).ToNot(HaveOccurred())

					typed, ok := ev.Event.(revaevents.SpaceEnabled)
					Expect(ok).To(BeTrue(), "expected SpaceEnabled, got %T", ev.Event)
					Expect(typed.Executant).ToNot(BeNil())
					Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
					Expect(typed.ID).ToNot(BeNil())
				})

				// A space must be disabled (soft-deleted) before it can be purged.
				It("emits SpaceDeleted with correct fields", func() {
					res, err := spacesClient.DeleteStorageSpace(ctx, &storagep.DeleteStorageSpaceRequest{
						Id: projID,
						Opaque: &typespb.Opaque{
							Map: map[string]*typespb.OpaqueEntry{
								"purge": {Decoder: "plain", Value: []byte("true")},
							},
						},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

					ev, err := consumeEvent(eventCh, 3*time.Second)
					Expect(err).ToNot(HaveOccurred())

					typed, ok := ev.Event.(revaevents.SpaceDeleted)
					Expect(ok).To(BeTrue(), "expected SpaceDeleted, got %T", ev.Event)
					Expect(typed.Executant).ToNot(BeNil())
					Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
					Expect(typed.ID).ToNot(BeNil())
					Expect(typed.SpaceName).To(Equal("projectspace"))
					Expect(typed.FinalMembers).To(HaveLen(1))
					ownerPerms, ok := typed.FinalMembers[ownerID] //nolint:govet
					Expect(ok).To(BeTrue(), "expected ownerID %q in FinalMembers", ownerID)
					Expect(ownerPerms.AddGrant).To(BeTrue())
				})
			})
		})

		Context("space sharing", func() {
			var (
				granteeUser = &userpb.UserId{
					Idp:      "0.0.0.0:19000",
					OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
					Type:     userpb.UserType_USER_TYPE_PRIMARY,
				}
				spaceRef = &storagep.Reference{
					ResourceId: &storagep.ResourceId{
						StorageId: spaceID,
						SpaceId:   spaceID,
						OpaqueId:  spaceID,
					},
				}
				grant = &storagep.Grant{
					Grantee: &storagep.Grantee{
						Type: storagep.GranteeType_GRANTEE_TYPE_USER,
						Id:   &storagep.Grantee_UserId{UserId: granteeUser},
					},
					Permissions: &storagep.ResourcePermissions{GetPath: true},
				}
				spaceGrantOpaque = &typespb.Opaque{
					Map: map[string]*typespb.OpaqueEntry{
						"spacegrant": {Decoder: "plain", Value: []byte("true")},
					},
				}
			)

			// SpaceShared
			It("emits SpaceShared with correct fields", func() {
				res, err := providerClient.AddGrant(ctx, &storagep.AddGrantRequest{
					Ref:    spaceRef,
					Grant:  grant,
					Opaque: spaceGrantOpaque,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.SpaceShared)
				Expect(ok).To(BeTrue(), "expected SpaceShared, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.Executant.OpaqueId).To(Equal(ownerID))
				Expect(typed.GranteeUserID).ToNot(BeNil())
				Expect(typed.GranteeUserID.OpaqueId).To(Equal(granteeUser.OpaqueId))
				Expect(typed.ID).ToNot(BeNil())
			})

			// SpaceShareUpdated
			It("emits SpaceShareUpdated with correct fields", func() {
				_, err := providerClient.AddGrant(ctx, &storagep.AddGrantRequest{
					Ref: spaceRef, Grant: grant, Opaque: spaceGrantOpaque,
				})
				Expect(err).ToNot(HaveOccurred())
				mustDrainEvent(eventCh)

				res, err := providerClient.UpdateGrant(ctx, &storagep.UpdateGrantRequest{
					Ref: spaceRef,
					Grant: &storagep.Grant{
						Grantee:     grant.Grantee,
						Permissions: &storagep.ResourcePermissions{GetPath: true, Stat: true},
					},
					Opaque: spaceGrantOpaque,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.SpaceShareUpdated)
				Expect(ok).To(BeTrue(), "expected SpaceShareUpdated, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.GranteeUserID).ToNot(BeNil())
				Expect(typed.ID).ToNot(BeNil())
			})

			// SpaceUnshared
			It("emits SpaceUnshared with correct fields", func() {
				_, err := providerClient.AddGrant(ctx, &storagep.AddGrantRequest{
					Ref: spaceRef, Grant: grant, Opaque: spaceGrantOpaque,
				})
				Expect(err).ToNot(HaveOccurred())
				mustDrainEvent(eventCh)

				res, err := providerClient.RemoveGrant(ctx, &storagep.RemoveGrantRequest{
					Ref:    spaceRef,
					Grant:  grant,
					Opaque: spaceGrantOpaque,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Status.Code).To(Equal(rpcv1beta1.Code_CODE_OK))

				ev, err := consumeEvent(eventCh, 3*time.Second)
				Expect(err).ToNot(HaveOccurred())

				typed, ok := ev.Event.(revaevents.SpaceUnshared)
				Expect(ok).To(BeTrue(), "expected SpaceUnshared, got %T", ev.Event)
				Expect(typed.Executant).ToNot(BeNil())
				Expect(typed.GranteeUserID).ToNot(BeNil())
				Expect(typed.ID).ToNot(BeNil())
			})
		})
	})
})
