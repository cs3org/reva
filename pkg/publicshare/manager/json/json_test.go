// Copyright 2018-2022 CERN
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

package json_test

import (
	"context"
	encjson "encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/publicshare"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json"
	"github.com/owncloud/reva/v2/pkg/publicshare/manager/json/persistence/cs3"
	"github.com/owncloud/reva/v2/pkg/rgrpc/status"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/owncloud/reva/v2/pkg/storage/utils/metadata"
	"github.com/owncloud/reva/v2/pkg/utils"
	"github.com/owncloud/reva/v2/tests/cs3mocks/mocks"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Json", func() {
	var (
		user1 = &userpb.User{
			Id: &userpb.UserId{
				Idp:      "https://localhost:9200",
				OpaqueId: "admin",
			},
		}

		sharedResource = &providerv1beta1.ResourceInfo{
			Id: &providerv1beta1.ResourceId{
				StorageId: "storageid",
				OpaqueId:  "opaqueid",
			},
			ArbitraryMetadata: &providerv1beta1.ArbitraryMetadata{
				Metadata: map[string]string{
					"name": "publicshare",
				},
			},
		}
		grant = &link.Grant{
			Permissions: &link.PublicSharePermissions{
				Permissions: &providerv1beta1.ResourcePermissions{
					InitiateFileUpload: false,
				},
			},
		}

		m       publicshare.Manager
		tmpFile *os.File
		ctx     context.Context
		client  *mocks.GatewayAPIClient
	)

	Context("with a file persistence layer", func() {

		BeforeEach(func() {
			var err error
			tmpFile, err = os.CreateTemp("", "reva-unit-test-*.json")
			Expect(err).ToNot(HaveOccurred())

			config := map[string]interface{}{
				"file":         tmpFile.Name(),
				"gateway_addr": "https://localhost:9200",
			}

			pool.RemoveSelector("GatewaySelector" + "https://localhost:9200")
			client = &mocks.GatewayAPIClient{}
			pool.GetSelector[gatewayv1beta1.GatewayAPIClient](
				"GatewaySelector",
				"https://localhost:9200",
				func(cc grpc.ClientConnInterface) gatewayv1beta1.GatewayAPIClient { return client },
			)

			m, err = json.NewFile(config)
			Expect(err).ToNot(HaveOccurred())

			ctx = ctxpkg.ContextSetUser(context.Background(), user1)
		})

		AfterEach(func() {
			os.Remove(tmpFile.Name())
		})

		Describe("Dump", func() {
			JustBeforeEach(func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{
					Password: "foo",
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("dumps all public shares", func() {
				psharesChan := make(chan *publicshare.WithPassword)
				pshares := []*publicshare.WithPassword{}

				wg := sync.WaitGroup{}
				wg.Add(1)
				go func() {
					for ps := range psharesChan {
						if ps != nil {
							pshares = append(pshares, ps)
						}
					}
					wg.Done()
				}()
				err := m.(publicshare.DumpableManager).Dump(ctx, psharesChan)
				Expect(err).ToNot(HaveOccurred())
				close(psharesChan)
				wg.Wait()
				Eventually(psharesChan).Should(BeClosed())

				Expect(len(pshares)).To(Equal(1))
				Expect(bcrypt.CompareHashAndPassword([]byte(pshares[0].Password), []byte("foo"))).To(Succeed())
				Expect(pshares[0].PublicShare.Creator).To(BeComparableTo(user1.Id, protocmp.Transform()))
				Expect(pshares[0].PublicShare.ResourceId).To(BeComparableTo(sharedResource.Id, protocmp.Transform()))
			})
		})

		Describe("ListPublicShares", func() {
			type shareSpec struct {
				ID, Token                    string
				Creator                      *userpb.UserId
				StorageID, SpaceID, OpaqueID string
			}

			seedShares := func(path string, specs []shareSpec) {
				raw, err := os.ReadFile(path)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				db := map[string]interface{}{}
				if len(raw) > 0 {
					ExpectWithOffset(1, encjson.Unmarshal(raw, &db)).To(Succeed())
				}

				for _, s := range specs {
					ps := &link.PublicShare{
						Id:         &link.PublicShareId{OpaqueId: s.ID},
						Token:      s.Token,
						Creator:    s.Creator,
						Owner:      s.Creator,
						ResourceId: &providerv1beta1.ResourceId{StorageId: s.StorageID, SpaceId: s.SpaceID, OpaqueId: s.OpaqueID},
					}
					enc, err := utils.MarshalProtoV1ToJSON(ps)
					ExpectWithOffset(1, err).ToNot(HaveOccurred())
					db[s.ID] = map[string]interface{}{"share": string(enc), "password": ""}
				}
				patched, err := encjson.Marshal(db)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, os.WriteFile(path, patched, 0644)).To(Succeed())
			}

			opaqueIDs := func(shares []*link.PublicShare) []string {
				out := make([]string, 0, len(shares))
				for _, s := range shares {
					out = append(out, s.Id.OpaqueId)
				}
				return out
			}

			It("skips shares whose persisted resource_id is nil instead of panicking", func() {
				// Create one valid share so the manager has a healthy row to compare against.
				validShare, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{
					Permissions: &link.PublicSharePermissions{
						Permissions: &providerv1beta1.ResourcePermissions{},
					},
				})
				Expect(err).ToNot(HaveOccurred())

				// Inject a corrupt row directly into the persistence file: the share's stored
				// JSON has no `resource_id`, so after unmarshal `local.ResourceId` is nil.
				// This mirrors the production state described in OCISDEV-862.
				raw, err := os.ReadFile(tmpFile.Name())
				Expect(err).ToNot(HaveOccurred())

				db := map[string]interface{}{}
				Expect(encjson.Unmarshal(raw, &db)).To(Succeed())

				db["corrupt-share-id"] = map[string]interface{}{
					"share":    `{"id":{"opaque_id":"corrupt-share-id"},"token":"corrupt-token"}`,
					"password": "",
				}
				patched, err := encjson.Marshal(db)
				Expect(err).ToNot(HaveOccurred())
				Expect(os.WriteFile(tmpFile.Name(), patched, 0644)).To(Succeed())

				// Listing must not panic and must return the valid share.
				shares, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(shares)).To(Equal(1))
				Expect(shares[0].Id.OpaqueId).To(Equal(validShare.Id.OpaqueId))
			})

			// Headline correctness property (OCISDEV-861): the returned set must be
			// exactly own shares plus foreign shares the caller may list grants on,
			// per the *unchanged* per-resource Stat check. A resource-narrowing
			// filter that still matches every seeded share must not change that:
			// there is no more special-casing between filtered and unfiltered
			// requests, both go through the very same statForeignResources path.
			It("returns exactly own shares plus permitted foreign shares, identically whether or not a resource filter is passed", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				ridPermitted := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-a", OpaqueId: "oa"}
				ridDenied := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-b", OpaqueId: "ob"}
				seedShares(tmpFile.Name(), []shareSpec{
					{ID: "own-1", Token: "t-own-1", Creator: user1.Id, StorageID: "storageid", SpaceID: "space-own", OpaqueID: "oown"},
					{ID: "foreign-permitted", Token: "t-fp", Creator: user2, StorageID: ridPermitted.StorageId, SpaceID: ridPermitted.SpaceId, OpaqueID: ridPermitted.OpaqueId},
					{ID: "foreign-denied", Token: "t-fd", Creator: user2, StorageID: ridDenied.StorageId, SpaceID: ridDenied.SpaceId, OpaqueID: ridDenied.OpaqueId},
				})

				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *providerv1beta1.StatRequest) bool {
					return utils.ResourceIDEqual(req.GetRef().GetResourceId(), ridPermitted)
				})).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
				}, nil)
				client.On("Stat", mock.Anything, mock.MatchedBy(func(req *providerv1beta1.StatRequest) bool {
					return utils.ResourceIDEqual(req.GetRef().GetResourceId(), ridDenied)
				})).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: false}},
				}, nil)

				unfiltered, err := m.ListPublicShares(ctx, user1, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(opaqueIDs(unfiltered)).To(ConsistOf("own-1", "foreign-permitted"))

				// Every seeded share lives under storage "storageid", so this filter
				// narrows nothing away; it exists purely to prove filtered and
				// unfiltered requests are decided identically.
				filtered, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{
					publicshare.StorageIDFilter("storageid"),
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(opaqueIDs(filtered)).To(ConsistOf("own-1", "foreign-permitted"))
			})

			It("stats each distinct resource at most once regardless of how many links point at it", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				const numResources = 5
				const numLinks = 500
				specs := make([]shareSpec, 0, numLinks)
				for i := 0; i < numLinks; i++ {
					specs = append(specs, shareSpec{
						ID:        fmt.Sprintf("foreign-%d", i),
						Token:     fmt.Sprintf("t-foreign-%d", i),
						Creator:   user2,
						StorageID: "storageid",
						SpaceID:   "space-foreign",
						OpaqueID:  fmt.Sprintf("o-%d", i%numResources),
					})
				}
				seedShares(tmpFile.Name(), specs)

				client.On("Stat", mock.Anything, mock.Anything).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
				}, nil)

				shares, err := m.ListPublicShares(ctx, user1, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(HaveLen(numLinks))
				// The point of the ticket: N links on M distinct resources costs M
				// stats, not N.
				client.AssertNumberOfCalls(GinkgoT(), "Stat", numResources)
			})

			It("bounds the number of concurrent Stat calls to maxConcurrency", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				const numResources = 10
				specs := make([]shareSpec, 0, numResources)
				for i := 0; i < numResources; i++ {
					specs = append(specs, shareSpec{
						ID:        fmt.Sprintf("foreign-%d", i),
						Token:     fmt.Sprintf("t-foreign-%d", i),
						Creator:   user2,
						StorageID: "storageid",
						SpaceID:   "space-foreign",
						OpaqueID:  fmt.Sprintf("o-%d", i),
					})
				}
				seedShares(tmpFile.Name(), specs)

				var mu sync.Mutex
				var inFlight, maxInFlight int
				client.On("Stat", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
					mu.Lock()
					inFlight++
					if inFlight > maxInFlight {
						maxInFlight = inFlight
					}
					mu.Unlock()

					time.Sleep(20 * time.Millisecond)

					mu.Lock()
					inFlight--
					mu.Unlock()
				}).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
				}, nil)

				bounded, err := json.NewFile(map[string]interface{}{
					"file":         tmpFile.Name(),
					"gateway_addr": "https://localhost:9200",
				})
				Expect(err).ToNot(HaveOccurred())
				json.SetStatConcurrency(bounded, 2)

				shares, err := bounded.ListPublicShares(ctx, user1, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(HaveLen(numResources))

				mu.Lock()
				defer mu.Unlock()
				// Never more than maxConcurrency Stat calls in flight at once...
				Expect(maxInFlight).To(BeNumerically("<=", 2))
				// ...but concurrency is actually happening, not accidentally serial.
				Expect(maxInFlight).To(BeNumerically(">=", 2))
			})

			It("fails closed when the caller's deadline leaves no budget at all", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				seedShares(tmpFile.Name(), []shareSpec{
					{ID: "own-1", Token: "t-own-1", Creator: user1.Id, StorageID: "storageid", SpaceID: "space-own", OpaqueID: "oown"},
					{ID: "foreign-1", Token: "t-f1", Creator: user2, StorageID: "storageid", SpaceID: "space-x", OpaqueID: "o1"},
				})

				// A Stat that takes far longer than the caller's deadline, but still
				// honours context cancellation the way a real gRPC client would. It
				// must never actually be invoked below: see the assertion at the end.
				client.On("Stat", mock.Anything, mock.Anything).Return(
					func(ctx context.Context, _ *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
						select {
						case <-time.After(50 * time.Millisecond):
							return &providerv1beta1.StatResponse{
								Status: status.NewOK(ctx),
								Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
							}, nil
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					})

				// statBudgetContext computes time.Until(deadline) - margin, and the
				// 200ms margin alone dwarfs this 5ms deadline, so the derived budget
				// clamps to zero: the fan-out starts with an already-expired context,
				// and every worker takes the gctx.Err() != nil branch on its very
				// first job, before ever calling Stat. This is the "no budget at all"
				// case; see the spec below for a genuine mid-flight cancellation.
				shortCtx, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
				defer cancel()

				start := time.Now()
				shares, err := m.ListPublicShares(shortCtx, user1, nil, false)
				elapsed := time.Since(start)

				// Partial list beats code = Canceled: no error...
				Expect(err).ToNot(HaveOccurred())
				// ...own shares are unaffected by the budget...
				Expect(opaqueIDs(shares)).To(ConsistOf("own-1"))
				// ...and a resource whose permission was never determined is never
				// included (fail closed).
				Expect(opaqueIDs(shares)).ToNot(ContainElement("foreign-1"))
				// The call must not block for the full 50ms Stat delay.
				Expect(elapsed).To(BeNumerically("<", 40*time.Millisecond))
				// The budget was already exhausted on entry, so Stat is never called.
				client.AssertNumberOfCalls(GinkgoT(), "Stat", 0)
			})

			It("fails closed and returns a partial list when the caller's own deadline expires mid-flight", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				ridA := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-x", OpaqueId: "o1"}
				ridB := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-x", OpaqueId: "o2"}
				ridC := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-x", OpaqueId: "o3"}
				seedShares(tmpFile.Name(), []shareSpec{
					{ID: "own-1", Token: "t-own-1", Creator: user1.Id, StorageID: "storageid", SpaceID: "space-own", OpaqueID: "oown"},
					{ID: "foreign-a", Token: "t-fa", Creator: user2, StorageID: ridA.StorageId, SpaceID: ridA.SpaceId, OpaqueID: ridA.OpaqueId},
					{ID: "foreign-b", Token: "t-fb", Creator: user2, StorageID: ridB.StorageId, SpaceID: ridB.SpaceId, OpaqueID: ridB.OpaqueId},
					{ID: "foreign-c", Token: "t-fc", Creator: user2, StorageID: ridC.StorageId, SpaceID: ridC.SpaceId, OpaqueID: ridC.OpaqueId},
				})

				// A single worker, so the three distinct resources are stated strictly
				// one after another rather than all at once. Each Stat costs ~120ms
				// against a residual budget of ~200ms (see below), so the first
				// resource is decided and the rest are cut off. That asymmetry is the
				// point: this spec pins a genuinely PARTIAL result, where the "no
				// budget at all" spec above pins the degenerate case in which nothing
				// is decided.
				//
				// Note on what enforces this: cancellation is honoured by the Stat
				// call itself (a real gRPC client aborts on a cancelled context, as
				// the mock below does). The worker's own gctx check is an
				// optimisation that avoids dispatching doomed RPCs, so removing it
				// does not change the outcome here. The property that actually keeps
				// this fail-closed is that an undecided resource is absent from the
				// permitted map — pinned by the "excludes a foreign share when Stat
				// fails closed" table below.
				json.SetStatConcurrency(m, 1)

				client.On("Stat", mock.Anything, mock.Anything).Return(
					func(ctx context.Context, _ *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
						select {
						case <-time.After(120 * time.Millisecond):
							return &providerv1beta1.StatResponse{
								Status: status.NewOK(ctx),
								Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
							}, nil
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					})

				// Residual budget is roughly 400ms - 200ms margin = ~200ms, so one or
				// two of the ~120ms Stats complete and the remainder are abandoned.
				midCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
				defer cancel()

				start := time.Now()
				shares, err := m.ListPublicShares(midCtx, user1, nil, false)
				elapsed := time.Since(start)
				returned := opaqueIDs(shares)

				// Partial list beats code = Canceled: no error...
				Expect(err).ToNot(HaveOccurred())
				// ...own shares are never subject to the budget...
				Expect(returned).To(ContainElement("own-1"))
				// ...at least one foreign share WAS decided permitted before the
				// budget ran out, so this is a partial result and not an empty one...
				foreign := 0
				for _, id := range returned {
					if id != "own-1" {
						foreign++
					}
				}
				Expect(foreign).To(BeNumerically(">", 0), "expected at least one foreign share to be decided before the budget expired")
				// ...but not all of them: the remainder were abandoned undecided and
				// are therefore excluded (fail closed, never fail open).
				Expect(foreign).To(BeNumerically("<", 3), "expected the budget to cut the stat fan-out short")
				// The call must not block for all three Stat delays.
				Expect(elapsed).To(BeNumerically("<", 360*time.Millisecond))
			})

			It("does not truncate the list when the caller grants a generous deadline", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				const numResources = 5
				specs := make([]shareSpec, 0, numResources)
				for i := 0; i < numResources; i++ {
					specs = append(specs, shareSpec{
						ID:        fmt.Sprintf("foreign-%d", i),
						Token:     fmt.Sprintf("t-foreign-%d", i),
						Creator:   user2,
						StorageID: "storageid",
						SpaceID:   "space-foreign",
						OpaqueID:  fmt.Sprintf("o-%d", i),
					})
				}
				seedShares(tmpFile.Name(), specs)

				// Each Stat is slow enough that an invented internal cap (e.g. the
				// old hard-coded 5s) would be indistinguishable from "no cap" at
				// this scale if one were still silently applied; what this proves
				// is that a generous caller deadline is honoured in full, not that
				// any particular duration is safe.
				client.On("Stat", mock.Anything, mock.Anything).Return(
					func(ctx context.Context, _ *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
						select {
						case <-time.After(50 * time.Millisecond):
							return &providerv1beta1.StatResponse{
								Status: status.NewOK(ctx),
								Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
							}, nil
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					})

				generousCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				shares, err := m.ListPublicShares(generousCtx, user1, nil, false)
				Expect(err).ToNot(HaveOccurred())
				// No artificial cap applies: every foreign share is present.
				Expect(opaqueIDs(shares)).To(ConsistOf("foreign-0", "foreign-1", "foreign-2", "foreign-3", "foreign-4"))
			})

			It("costs zero Stat calls when the caller created every share", func() {
				const numOwn = 10
				specs := make([]shareSpec, 0, numOwn)
				for i := 0; i < numOwn; i++ {
					specs = append(specs, shareSpec{
						ID:        fmt.Sprintf("own-%d", i),
						Token:     fmt.Sprintf("t-own-%d", i),
						Creator:   user1.Id,
						StorageID: "storageid",
						SpaceID:   "space-own",
						OpaqueID:  fmt.Sprintf("o-%d", i),
					})
				}
				seedShares(tmpFile.Name(), specs)

				shares, err := m.ListPublicShares(ctx, user1, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(HaveLen(numOwn))
				client.AssertNumberOfCalls(GinkgoT(), "Stat", 0)
			})

			It("still stats when a resource filter is given", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				rid := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-x", OpaqueId: "o1"}
				seedShares(tmpFile.Name(), []shareSpec{
					{ID: "foreign-1", Token: "t-f1", Creator: user2, StorageID: "storageid", SpaceID: "space-x", OpaqueID: "o1"},
				})
				client.On("Stat", mock.Anything, mock.Anything).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: true}},
				}, nil)

				shares, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{publicshare.ResourceIDFilter(rid)}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(HaveLen(1))
				client.AssertNumberOfCalls(GinkgoT(), "Stat", 1)
			})

			It("stats a denied resource only once even with several shares on it", func() {
				user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
				rid := &providerv1beta1.ResourceId{StorageId: "storageid", SpaceId: "space-x", OpaqueId: "o1"}
				seedShares(tmpFile.Name(), []shareSpec{
					{ID: "f1", Token: "t-f1", Creator: user2, StorageID: "storageid", SpaceID: "space-x", OpaqueID: "o1"},
					{ID: "f2", Token: "t-f2", Creator: user2, StorageID: "storageid", SpaceID: "space-x", OpaqueID: "o1"},
				})
				client.On("Stat", mock.Anything, mock.Anything).Return(&providerv1beta1.StatResponse{
					Status: status.NewOK(ctx),
					Info:   &providerv1beta1.ResourceInfo{PermissionSet: &providerv1beta1.ResourcePermissions{ListGrants: false}},
				}, nil)

				shares, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{publicshare.ResourceIDFilter(rid)}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(BeEmpty())
				client.AssertNumberOfCalls(GinkgoT(), "Stat", 1) // deduplicated to a single Stat call by the pass-1 resource map
			})

			// userCanListGrants fails closed (excludes the share, records false) for
			// every one of these outcomes. None of them had a spec before: a
			// regression flipping any one to fail-open (e.g. recording true on an
			// error) would not have failed a single test.
			DescribeTable("excludes a foreign share when Stat fails closed",
				func(statFn func(ctx context.Context) (*providerv1beta1.StatResponse, error)) {
					user2 := &userpb.UserId{Idp: "https://localhost:9200", OpaqueId: "einstein"}
					seedShares(tmpFile.Name(), []shareSpec{
						{ID: "own-1", Token: "t-own-1", Creator: user1.Id, StorageID: "storageid", SpaceID: "space-own", OpaqueID: "oown"},
						{ID: "foreign-1", Token: "t-f1", Creator: user2, StorageID: "storageid", SpaceID: "space-x", OpaqueID: "o1"},
					})

					client.On("Stat", mock.Anything, mock.Anything).Return(
						func(ctx context.Context, _ *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
							return statFn(ctx)
						})

					shares, err := m.ListPublicShares(ctx, user1, nil, false)
					Expect(err).ToNot(HaveOccurred())
					// The own share proves this is exclusion of the foreign share, not a
					// blanket empty result from some unrelated failure.
					Expect(opaqueIDs(shares)).To(ConsistOf("own-1"))
					Expect(opaqueIDs(shares)).ToNot(ContainElement("foreign-1"))
				},
				Entry("transport error", func(_ context.Context) (*providerv1beta1.StatResponse, error) {
					return nil, errors.New("transport error talking to the gateway")
				}),
				Entry("CODE_NOT_FOUND", func(ctx context.Context) (*providerv1beta1.StatResponse, error) {
					return &providerv1beta1.StatResponse{
						Status: status.NewNotFound(ctx, "resource not found"),
					}, nil
				}),
				Entry("other non-OK status", func(ctx context.Context) (*providerv1beta1.StatResponse, error) {
					return &providerv1beta1.StatResponse{
						Status: status.NewInternal(ctx, "internal error"),
					}, nil
				}),
				Entry("OK with nil Info", func(ctx context.Context) (*providerv1beta1.StatResponse, error) {
					return &providerv1beta1.StatResponse{
						Status: status.NewOK(ctx),
						Info:   nil,
					}, nil
				}),
				Entry("OK with Info but nil PermissionSet", func(ctx context.Context) (*providerv1beta1.StatResponse, error) {
					return &providerv1beta1.StatResponse{
						Status: status.NewOK(ctx),
						Info:   &providerv1beta1.ResourceInfo{PermissionSet: nil},
					}, nil
				}),
			)
		})

		Describe("Load", func() {
			It("loads shares including state and mountpoint information", func() {
				existingShare, err := m.CreatePublicShare(ctx, user1, sharedResource, &link.Grant{
					Password: "foo",
				})
				Expect(err).ToNot(HaveOccurred())

				targetManager, err := json.NewMemory(map[string]interface{}{})
				Expect(err).ToNot(HaveOccurred())

				sharesChan := make(chan *publicshare.WithPassword)

				wg := sync.WaitGroup{}
				wg.Add(2)
				go func() {
					err := targetManager.(publicshare.LoadableManager).Load(ctx, sharesChan)
					Expect(err).ToNot(HaveOccurred())
					wg.Done()
				}()
				go func() {
					tmpShare := &publicshare.WithPassword{
						Password: "foo",
					}
					proto.Merge(&tmpShare.PublicShare, existingShare)
					sharesChan <- tmpShare
					close(sharesChan)
					wg.Done()
				}()
				wg.Wait()
				Eventually(sharesChan).Should(BeClosed())

				loadedPublicShare, err := targetManager.GetPublicShare(ctx, user1, &link.PublicShareReference{
					Spec: &link.PublicShareReference_Token{
						Token: existingShare.Token,
					},
				}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedPublicShare).ToNot(BeNil())
			})
		})
	})

	Context("with a cs3 persistence layer", func() {
		var (
			tmpdir string

			storage metadata.Storage
		)

		BeforeEach(func() {
			var err error
			tmpdir, err = os.MkdirTemp("", "json-publicshare-manager-test")
			Expect(err).ToNot(HaveOccurred())

			err = os.MkdirAll(tmpdir, 0755)
			Expect(err).ToNot(HaveOccurred())

			storage, err = metadata.NewDiskStorage(tmpdir)
			Expect(err).ToNot(HaveOccurred())

			persistence := cs3.New(storage)
			Expect(persistence.Init(context.Background())).To(Succeed())

			m, err = json.New("https://localhost:9200", 11, 60, false, persistence)
			Expect(err).ToNot(HaveOccurred())

			ctx = ctxpkg.ContextSetUser(context.Background(), user1)
		})

		AfterEach(func() {
			if tmpdir != "" {
				os.RemoveAll(tmpdir)
			}
		})
		Describe("CreatePublicShare", func() {
			It("creates public shares", func() {
				ps, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())
				Expect(ps).ToNot(BeNil())
			})

			It("rejects a resource info with a nil resource id instead of persisting it", func() {
				noID := &providerv1beta1.ResourceInfo{
					ArbitraryMetadata: &providerv1beta1.ArbitraryMetadata{
						Metadata: map[string]string{"name": "publicshare"},
					},
				}

				ps, err := m.CreatePublicShare(ctx, user1, noID, grant)
				Expect(err).To(HaveOccurred())
				Expect(ps).To(BeNil())

				shares, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(shares).To(BeEmpty())
			})
		})

		Describe("PublicShares", func() {
			It("lists public shares", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))
				Expect(ps[0].ResourceId).To(Equal(sharedResource.Id))
			})

			It("picks up shares from the storage", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				// Reset manager
				p := cs3.New(storage)
				Expect(p.Init(context.Background())).To(Succeed())

				m, err = json.New("https://localhost:9200", 11, 60, false, p)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))
				Expect(ps[0].ResourceId).To(Equal(sharedResource.Id))
			})

			It("refreshes its cache before writing new data", func() {
				_, err := m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err := m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1))

				// Purge file on storage and make sure its mtime is newer than the cache
				path := filepath.Join(tmpdir, "publicshares.json")
				Expect(os.WriteFile(path, []byte("{}"), 0x644)).To(Succeed())
				t := time.Now().Add(5 * time.Minute)
				Expect(os.Chtimes(path, t, t)).To(Succeed())

				_, err = m.CreatePublicShare(ctx, user1, sharedResource, grant)
				Expect(err).ToNot(HaveOccurred())

				ps, err = m.ListPublicShares(ctx, user1, []*link.ListPublicSharesRequest_Filter{}, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(ps)).To(Equal(1)) // Make sure the first created public share is gone
			})
		})
	})
})
