// Copyright 2018-2024 CERN
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

package spacesregistry

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/projects"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// statGateway knows only the given paths, and keeps the paths that it was asked to stat.
type statGateway struct {
	gateway.GatewayAPIClient

	infos map[string]*provider.ResourceInfo
	stats []string
}

func (g *statGateway) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	g.stats = append(g.stats, req.Ref.GetPath())
	info, ok := g.infos[req.Ref.GetPath()]
	if !ok {
		return &provider.StatResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_PERMISSION_DENIED}}, nil
	}
	return &provider.StatResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, Info: info}, nil
}

func (g *statGateway) GetQuota(context.Context, *gateway.GetQuotaRequest, ...grpc.CallOption) (*provider.GetQuotaResponse, error) {
	return &provider.GetQuotaResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil
}

// testClients is installed one time for all tests, because revaservice.SetGlobal keeps the first clients.
type testClients struct {
	revaservice.Clients
	gw gateway.GatewayAPIClient
}

var clients = &testClients{}

func (c *testClients) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	return c.gw, nil
}

// noProjects finds no project, like the catalogue for a user who is not a member.
type noProjects struct {
	projects.Catalogue
}

func (noProjects) ListStorageSpaces(context.Context, *provider.ListStorageSpacesRequest, projects.ProjectStatus) (*provider.ListStorageSpacesResponse, error) {
	return &provider.ListStorageSpacesResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil
}

func TestListStorageSpacesByIDStatsOnlyTheSpaceOfTheID(t *testing.T) {
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "prefectt", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "prefectt"}
	ctx := appctx.ContextSetUser(context.Background(), user)

	home := "/eos/user/p/prefectt"
	rootInfo := func(storageID, path string) *provider.ResourceInfo {
		return &provider.ResourceInfo{Id: &provider.ResourceId{StorageId: storageID, SpaceId: spaces.EncodeSpaceID(path)}, Path: path}
	}
	revaservice.SetGlobal(clients)

	for name, tc := range map[string]struct {
		id         string
		wantStats  []string
		wantSpaces int
	}{
		"a project that the user cannot see": {
			id: spaces.EncodeSpaceID("/eos/project/o/proj"),
		},
		"an ID that holds no path": {
			id: "not-a-space-id",
		},
		"the home of the user": {
			id:         spaces.EncodeStorageSpaceID("eoshome", spaces.EncodeSpaceID(home)),
			wantStats:  []string{home},
			wantSpaces: 1,
		},
		"a public space": {
			id:         spaces.EncodeStorageSpaceID("eosams", spaces.EncodeSpaceID("/eos/ams")),
			wantStats:  []string{"/eos/ams"},
			wantSpaces: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// The user cannot stat /eos/atlas, so its stat is never cached.
			gw := &statGateway{infos: map[string]*provider.ResourceInfo{
				home:       rootInfo("eoshome", home),
				"/eos/ams": rootInfo("eosams", "/eos/ams"),
			}}
			clients.gw = gw
			riCache, err := memory.New[*provider.ResourceInfo](nil)
			require.NoError(t, err)
			s := &service{
				c: &config{
					UserSpace: "/eos/user/{{substr 0 1 .Username}}/{{.Username}}",
					PublicSpaces: map[string]map[string]string{
						"atlas": {"path": "/eos/atlas"},
						"ams":   {"path": "/eos/ams"},
					},
				},
				projects:          noProjects{},
				resourceInfoCache: riCache,
			}

			res, err := s.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
				Filters: spaces.ListStorageSpaceFilter{}.ByID(&provider.StorageSpaceId{OpaqueId: tc.id}).List(),
			})
			require.NoError(t, err)
			require.Equal(t, rpcv1beta1.Code_CODE_OK, res.Status.Code)
			assert.Len(t, res.StorageSpaces, tc.wantSpaces)
			assert.ElementsMatch(t, tc.wantStats, gw.stats)
		})
	}
}

// downGateway knows the alive paths. A request on a path that is down waits for
// the end of the context, like a request on an EOS instance that is down.
type downGateway struct {
	gateway.GatewayAPIClient

	alive, down []string
}

func (g *downGateway) wait(ctx context.Context, path string) error {
	if slices.ContainsFunc(g.down, func(d string) bool { return strings.HasPrefix(path, d) }) {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (g *downGateway) Stat(ctx context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	if err := g.wait(ctx, req.Ref.GetPath()); err != nil {
		return nil, err
	}
	if !slices.Contains(g.alive, req.Ref.GetPath()) {
		return &provider.StatResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_NOT_FOUND}}, nil
	}
	info := &provider.ResourceInfo{Id: &provider.ResourceId{StorageId: "eos", SpaceId: spaces.EncodeSpaceID(req.Ref.GetPath())}, Path: req.Ref.GetPath()}
	return &provider.StatResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, Info: info}, nil
}

func (g *downGateway) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest, _ ...grpc.CallOption) (*provider.GetQuotaResponse, error) {
	if err := g.wait(ctx, req.Ref.GetPath()); err != nil {
		return nil, err
	}
	return &provider.GetQuotaResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}}, nil
}

// someProjects finds the projects at the given paths.
type someProjects struct {
	projects.Catalogue

	paths []string
}

func (p someProjects) ListStorageSpaces(context.Context, *provider.ListStorageSpacesRequest, projects.ProjectStatus) (*provider.ListStorageSpacesResponse, error) {
	var sp []*provider.StorageSpace
	for _, path := range p.paths {
		sp = append(sp, &provider.StorageSpace{Name: path, SpaceType: spaces.SpaceTypeProject.AsString(), RootInfo: &provider.ResourceInfo{Path: path}})
	}
	return &provider.ListStorageSpacesResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, StorageSpaces: sp}, nil
}

func TestListProjectsWithStorageDown(t *testing.T) {
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "prefectt", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "prefectt"}
	ctx := appctx.ContextSetUser(context.Background(), user)

	const (
		alive   = "/eos/project/a/alive"
		down    = "/eos/project/d/down"
		missing = "/eos/project/m/missing"
	)
	for name, tc := range map[string]struct {
		projects       []string
		down           []string
		want           []string
		wantIncomplete bool
	}{
		"a project on a storage that is down is left out": {
			projects:       []string{alive, down},
			down:           []string{down},
			want:           []string{alive, "ams", "atlas"},
			wantIncomplete: true,
		},
		"a public space on a storage that is down is left out": {
			projects:       []string{alive},
			down:           []string{"/eos/atlas"},
			want:           []string{alive, "ams"},
			wantIncomplete: true,
		},
		"a project without a root does not make the list incomplete": {
			projects: []string{alive, missing},
			want:     []string{alive, "ams", "atlas"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			revaservice.SetGlobal(clients)
			clients.gw = &downGateway{alive: []string{alive, down, "/eos/ams", "/eos/atlas"}, down: tc.down}
			riCache, err := memory.New[*provider.ResourceInfo](nil)
			require.NoError(t, err)
			s := &service{
				c: &config{
					PublicSpaces: map[string]map[string]string{
						"atlas": {"path": "/eos/atlas"},
						"ams":   {"path": "/eos/ams"},
					},
				},
				projects:          someProjects{paths: tc.projects},
				resourceInfoCache: riCache,
				timeoutSkipSpaces: 100 * time.Millisecond,
			}

			type result struct {
				res *provider.ListStorageSpacesResponse
				err error
			}
			done := make(chan result, 1)
			go func() {
				res, err := s.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
					Filters: spaces.ListStorageSpaceFilter{}.BySpaceType(spaces.SpaceTypeProject).List(),
				})
				done <- result{res, err}
			}()
			var r result
			select {
			case r = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("ListStorageSpaces does not return while a storage is down")
			}

			require.NoError(t, r.err)
			require.Equal(t, rpcv1beta1.Code_CODE_OK, r.res.Status.Code)
			var names []string
			for _, sp := range r.res.StorageSpaces {
				names = append(names, sp.Name)
			}
			assert.ElementsMatch(t, tc.want, names)
			_, incomplete := r.res.GetOpaque().GetMap()[spaces.OpaqueIncomplete]
			assert.Equal(t, tc.wantIncomplete, incomplete)
		})
	}
}
