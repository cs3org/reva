// Copyright 2018-2023 CERN
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

package dynamic

import (
	"context"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registryv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"

	"github.com/cs3org/reva/pkg/storage"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var _ = Describe("Dynamic storage provider", func() {
	var (
		ctxAlice = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "alice",
			},
		})
		ctxBob = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "bob",
			},
		})
		ctxCharlie = ctxpkg.ContextSetUser(context.Background(), &userpb.User{
			Id: &userpb.UserId{
				OpaqueId: "charlie",
			},
		})

		routes = map[string]string{
			"/home-a":                   "eoshome-i01",
			"/home-b":                   "eoshome-i02",
			"/home-c":                   "eoshome-i03",
			"/eos/user/a":               "eosuser-i01",
			"/eos/user/b":               "eosuser-i02",
			"/eos/project/a":            "eosproject-i00",
			"/eos/project/c":            "eosproject-i02",
			"/cephfs/project/c/cephbox": "cephfs",
			"/cephfs/project/s/sebtest": "vaultssda01",
		}

		rules = map[string]string{
			"eoshome-i01":    "eoshome-i01:1234",
			"eoshome-i02":    "eoshome-i02:1234",
			"eosuser-i01":    "eosuser-i01:1234",
			"eosuser-i02":    "eosuser-i02:1234",
			"eosproject-i00": "eosproject-i00:1234",
			"eosproject-i02": "eosproject-i02:1234",
			"cephfs":         "cephfs:1234",
			"vaultssda01":    "vaultssda01:1234",
		}
		rewrites = map[string]string{
			"/home":   "/home-{{substr 0 1 .Id.OpaqueId}}",
			"/Shares": "/{{substr 0 1 .Id.OpaqueId}}",
		}
		homePath = "/home"

		d   storage.Registry
		h   *registryv1beta1.ProviderInfo
		err error

		providers = map[string]*registryv1beta1.ProviderInfo{
			"/home-a": {
				ProviderPath: "/home",
				Address:      "eoshome-i01:1234",
			},
			"/home-b": {
				ProviderPath: "/home",
				Address:      "eoshome-i02:1234",
			},
			"/eos/user/a": {
				ProviderPath: "/eos/user/a",
				Address:      "eosuser-i01:1234",
			},
			"/eos/user/b": {
				ProviderPath: "/eos/user/b",
				Address:      "eosuser-i02:1234",
			},
			"/eos/project/a": {
				ProviderPath: "/eos/project/a",
				Address:      "eosproject-i00:1234",
			},
			"/eos/project/c": {
				ProviderPath: "/eos/project/c",
				Address:      "eosproject-i02:1234",
			},
			"/cephfs/project/c/cephbox": {
				ProviderPath: "/cephfs/project/c/cephbox",
				Address:      "cephfs:1234",
			},
			"/cephfs/project/s/sebtest": {
				ProviderPath: "/cephfs/project/s/sebtest",
				Address:      "vaultssda01:1234",
			},
		}

		testHomeRefs = map[string]*provider.Reference{
			"/home": {
				Path: "/home",
			},
			"/home/test/a/deep/folder": {
				Path: "/home/test/a/deep/folder",
			},
		}

		testPaths = map[string][]*registryv1beta1.ProviderInfo{
			"/eos":                                 {providers["/eos/user/a"], providers["/eos/user/b"], providers["/eos/project/a"], providers["/eos/project/c"]},
			"/eos/user":                            {providers["/eos/user/a"], providers["/eos/user/b"]},
			"/eos/project":                         {providers["/eos/project/a"], providers["/eos/project/c"]},
			"/cephfs":                              {providers["/cephfs/project/c/cephbox"], providers["/cephfs/project/s/sebtest"]},
			"/eos/user/a/alice/test/a/deep/folder": {providers["/eos/user/a"]},
		}
	)

	BeforeSuite(func() {
		dbHost := "localhost"
		dbPort := 3306
		dbName := "reva_tests"
		sqlCtx := sql.NewEmptyContext()
		db := memory.NewDatabase(dbName)

		db.EnablePrimaryKeyIndexes()

		routingTable := memory.NewTable("routing", sql.NewPrimaryKeySchema(sql.Schema{
			{Name: "path", Type: sql.Text, Nullable: false, Source: "routing", PrimaryKey: true},
			{Name: "mount_id", Type: sql.Text, Nullable: false, Source: "routing"},
		}), &memory.ForeignKeyCollection{})

		must(routingTable.CreateIndex(sqlCtx, "idx_path", sql.IndexUsing_BTree, sql.IndexConstraint_Unique, []sql.IndexColumn{{Name: "path"}}, ""))

		for m, a := range routes {
			must(routingTable.Insert(sqlCtx, sql.NewRow(m, a)))
		}

		db.AddTable("routing", routingTable)

		config := server.Config{
			Protocol: "tcp",
			Address:  fmt.Sprintf("%s:%d", dbHost, dbPort),
		}

		engine := sqle.NewDefault(memory.NewMemoryDBProvider(db))
		s, err := server.NewDefaultServer(config, engine)
		if err != nil {
			panic(err)
		}

		go func() {
			if err := s.Start(); err != nil {
				panic(err)
			}
		}()

		d, err = New(context.Background(), map[string]interface{}{
			"rules":       rules,
			"rewrites":    rewrites,
			"home_path":   homePath,
			"db_username": "test",
			"db_password": "test",
			"db_host":     dbHost,
			"db_port":     dbPort,
			"db_name":     dbName,
		})
		if err != nil {
			panic(err)
		}

		if err := s.Close(); err != nil {
			panic(err)
		}
	})

	Context("initializing the provider", func() {
		When("passed correct config", func() {
			It("should return a correct dynamic provider", func() {
				Expect(d).ToNot(BeNil())
			})
		})
	})

	Context("listing providers", func() {
		It("should return a correct list of providers", func() {
			p, err := d.ListProviders(context.Background())
			Expect(p).To(HaveLen(len(providers)))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("getting home for user", func() {
		When("passed context for user alice", func() {
			It("should return the correct provider", func() {
				h, err = d.GetHome(ctxAlice)
				Expect(h).To(Equal(providers["/home-a"]))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("passed context for user bob", func() {
			It("should return the correct provider", func() {
				h, err = d.GetHome(ctxBob)
				Expect(h).To(Equal(providers["/home-b"]))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("passed context for user charlie who has home provider with a defined route but no rule in config", func() {
			It("should return a provider not found error", func() {
				h, err = d.GetHome(ctxCharlie)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(errtypes.InternalError("storage provider address not configured for mountID eoshome-i03")))
			})
		})

		When("passed context without user", func() {
			It("should return an error", func() {
				h, err = d.GetHome(context.Background())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("finding providers for a reference", func() {
		When("passed an storage id", func() {
			It("should return the correct provider", func() {
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{
						StorageId: "eoshome-i01",
					},
				}

				p, err := d.FindProviders(ctxAlice, ref)
				Expect(p).To(HaveLen(1))
				Expect(p[0].Address).To(Equal("eoshome-i01:1234"))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("passed a non-existing storage id and no path", func() {
			It("should return a provider not found error", func() {
				ref := &provider.Reference{
					ResourceId: &provider.ResourceId{
						StorageId: "nope",
					},
				}

				_, err := d.FindProviders(ctxAlice, ref)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(errtypes.NotFound("storage provider not found for ref resource_id:<storage_id:\"nope\" > ")))
			})
		})

		for u, ctx := range map[string]context.Context{"alice": ctxAlice, "bob": ctxBob} {
			u := u
			ctx := ctx
			for _, ref := range testHomeRefs {
				ref := ref
				When("passed a home path for user "+u+": "+ref.Path, func() {
					It("should return the correct provider", func() {
						ps, err := d.FindProviders(ctx, ref)
						Expect(err).ToNot(HaveOccurred())
						Expect(ps).To(HaveLen(1))
						Expect(ps[0].Address).To(Equal(providers["/home-"+string(u[0])].Address))
					})
				})
			}
		}

		for path, providers := range testPaths {
			path := path
			providers := providers
			When("passed a regular path: "+path, func() {
				It("should return the correct providers", func() {
					ps, err := d.FindProviders(context.Background(), &provider.Reference{Path: path})
					Expect(err).ToNot(HaveOccurred())
					Expect(ps).To(HaveLen(len(providers)))
					Expect(ps).To(ContainElements(providers))
				})
			})
		}

		When("passed a home path for user charlie who has home provider with a defined route but no rule in config", func() {
			It("should return a provider not found error", func() {
				_, err := d.FindProviders(ctxCharlie, testHomeRefs["/home"])
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(errtypes.InternalError("storage provider address not configured for mountID eoshome-i03")))
			})
		})

		When("passed a non-existing path", func() {
			It("should return a provider not found error", func() {
				_, err := d.FindProviders(ctxCharlie, &provider.Reference{
					Path: "/non/existent",
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(errtypes.NotFound("storage provider not found for ref path:\"/non/existent\" ")))
			})
		})
	})
})
