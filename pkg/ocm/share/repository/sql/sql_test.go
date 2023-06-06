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

package sql

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"

	"github.com/cs3org/reva/pkg/ocm/share"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/server"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/gdexlab/go-render/render"
)

var (
	dbName               = "reva_tests"
	address              = "localhost"
	port                 = 3306
	m                    sync.Mutex // for increasing the port
	ocmShareTable        = "ocm_shares"
	ocmAccessMethodTable = "ocm_shares_access_methods"
	ocmAMWebDAVTable     = "ocm_access_method_webdav"
	ocmAMWebappTable     = "ocm_access_method_webapp"

	ocmReceivedShareTable = "ocm_received_shares"
	ocmReceivedProtocols  = "ocm_received_share_protocols"
	ocmProtWebDAVTable    = "ocm_protocol_webdav"
	ocmProtWebappTable    = "ocm_protocol_webapp"
	ocmProtTransferTable  = "ocm_protocol_transfer"
)

func startDatabase(ctx *sql.Context, tables map[string]*memory.Table) (engine *sqle.Engine, p int, cleanup func()) {
	m.Lock()
	defer m.Unlock()

	db := memory.NewDatabase(dbName)
	db.EnablePrimaryKeyIndexes()
	for name, table := range tables {
		db.AddTable(name, table)
	}

	p = port
	config := server.Config{
		Protocol: "tcp",
		Address:  fmt.Sprintf("%s:%d", address, p),
	}
	port++
	engine = sqle.NewDefault(memory.NewMemoryDBProvider(db))
	s, err := server.NewDefaultServer(config, engine)
	if err != nil {
		panic(err)
	}

	go func() {
		if err := s.Start(); err != nil {
			panic(err)
		}
	}()
	cleanup = func() {
		if err := s.Close(); err != nil {
			panic(err)
		}
	}
	return
}

func getIDFunc() func() int64 {
	var i int64
	return func() int64 {
		i++
		return i
	}
}

func createShareTables(ctx *sql.Context, initData []*ocm.Share) map[string]*memory.Table {
	id := getIDFunc()
	tables := make(map[string]*memory.Table)

	// ocm_shares table
	tableShares := memory.NewTable(ocmShareTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: sql.Int64, Nullable: false, Source: ocmShareTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "token", Type: sql.Text, Nullable: false, Source: ocmShareTable, PrimaryKey: true},
		{Name: "fileid_prefix", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "item_source", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "name", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "share_with", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "owner", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "initiator", Type: sql.Text, Nullable: false, Source: ocmShareTable},
		{Name: "ctime", Type: sql.Uint64, Nullable: false, Source: ocmShareTable},
		{Name: "mtime", Type: sql.Uint64, Nullable: false, Source: ocmShareTable},
		{Name: "expiration", Type: sql.Uint64, Nullable: true, Source: ocmShareTable},
		{Name: "type", Type: sql.Int8, Nullable: false, Source: ocmShareTable},
	}), &memory.ForeignKeyCollection{})

	must(tableShares.CreateIndex(ctx, "test", sql.IndexUsing_BTree, sql.IndexConstraint_Unique, []sql.IndexColumn{
		{Name: "fileid_prefix"},
		{Name: "item_source"},
		{Name: "share_with"},
		{Name: "owner"},
	}, ""))
	tables[ocmShareTable] = tableShares

	// ocm_shares_access_methods table
	var fkAccessMethods memory.ForeignKeyCollection
	fkAccessMethods.AddFK(sql.ForeignKeyConstraint{
		Columns:       []string{"ocm_share_id"},
		ParentTable:   ocmShareTable,
		ParentColumns: []string{"id"},
		OnDelete:      sql.ForeignKeyReferentialAction_Cascade,
	})
	accessMethods := memory.NewTable(ocmAccessMethodTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: sql.Int64, Nullable: false, Source: ocmAccessMethodTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "ocm_share_id", Type: sql.Int64, Nullable: false, Source: ocmAccessMethodTable},
		{Name: "type", Type: sql.Int8, Nullable: false, Source: ocmAccessMethodTable},
	}), &fkAccessMethods)
	must(accessMethods.CreateIndex(ctx, "test", sql.IndexUsing_BTree, sql.IndexConstraint_Unique, []sql.IndexColumn{
		{Name: "ocm_share_id"},
		{Name: "type"},
	}, ""))
	tables[ocmAccessMethodTable] = accessMethods

	// ocm_access_method_webdav table
	var kfProtocols memory.ForeignKeyCollection
	kfProtocols.AddFK(sql.ForeignKeyConstraint{
		Columns:       []string{"ocm_access_method_id"},
		ParentTable:   ocmAccessMethodTable,
		ParentColumns: []string{"id"},
		OnDelete:      sql.ForeignKeyReferentialAction_Cascade,
	})

	webdav := memory.NewTable(ocmAMWebDAVTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "ocm_access_method_id", Type: sql.Int64, Nullable: false, Source: ocmAMWebDAVTable},
		{Name: "permissions", Type: sql.Int64, Nullable: false, Source: ocmAMWebDAVTable},
	}), &kfProtocols)
	tables[ocmAMWebDAVTable] = webdav

	// ocm_access_method_webapp table
	webapp := memory.NewTable(ocmAMWebappTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "ocm_access_method_id", Type: sql.Int64, Nullable: false, Source: ocmAMWebappTable},
		{Name: "view_mode", Type: sql.Int8, Nullable: false, Source: ocmAMWebappTable},
	}), &kfProtocols)
	tables[ocmAMWebappTable] = webapp

	for _, share := range initData {
		shareWith := share.Grantee.GetUserId()
		var expiration uint64
		if share.Expiration != nil {
			expiration = share.Expiration.Seconds
		}
		must(tableShares.Insert(ctx, sql.NewRow(mustInt(share.Id.OpaqueId), share.Token, share.ResourceId.StorageId, share.ResourceId.OpaqueId, share.Name, fmt.Sprintf("%s@%s", shareWith.OpaqueId, shareWith.Idp), share.Owner.OpaqueId, share.Creator.OpaqueId, share.Ctime.Seconds, share.Mtime.Seconds, expiration, int8(ShareTypeUser))))

		for _, m := range share.AccessMethods {
			i := id()
			switch am := m.Term.(type) {
			case *ocm.AccessMethod_WebdavOptions:
				must(accessMethods.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(WebDAVAccessMethod))))
				must(webdav.Insert(ctx, sql.NewRow(i, int64(conversions.RoleFromResourcePermissions(am.WebdavOptions.GetPermissions()).OCSPermissions()))))
			case *ocm.AccessMethod_WebappOptions:
				must(accessMethods.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(WebappAccessMethod))))
				must(webapp.Insert(ctx, sql.NewRow(i, int8(am.WebappOptions.ViewMode))))
			case *ocm.AccessMethod_TransferOptions:
				must(accessMethods.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(TransferAccessMethod))))
			}
		}
	}

	return tables
}

func createReceivedShareTables(ctx *sql.Context, initData []*ocm.ReceivedShare) map[string]*memory.Table {
	id := getIDFunc()
	tables := make(map[string]*memory.Table)

	// ocm_received_shares table
	tableShares := memory.NewTable(ocmReceivedShareTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: sql.Int64, Nullable: false, Source: ocmReceivedShareTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "name", Type: sql.Text, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "remote_share_id", Type: sql.Text, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "item_type", Type: sql.Int8, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "share_with", Type: sql.Text, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "owner", Type: sql.Text, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "initiator", Type: sql.Text, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "ctime", Type: sql.Uint64, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "mtime", Type: sql.Uint64, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "expiration", Type: sql.Uint64, Nullable: true, Source: ocmReceivedShareTable},
		{Name: "type", Type: sql.Int8, Nullable: false, Source: ocmReceivedShareTable},
		{Name: "state", Type: sql.Int8, Nullable: false, Source: ocmReceivedShareTable},
	}), &memory.ForeignKeyCollection{})
	tables[ocmReceivedShareTable] = tableShares

	// ocm_received_share_protocols table
	var fkAccessMethods memory.ForeignKeyCollection
	fkAccessMethods.AddFK(sql.ForeignKeyConstraint{
		Columns:       []string{"ocm_received_share_id"},
		ParentTable:   "ocm_received_shares",
		ParentColumns: []string{"id"},
		OnDelete:      sql.ForeignKeyReferentialAction_Cascade,
	})
	protocols := memory.NewTable(ocmReceivedProtocols, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "id", Type: sql.Int64, Nullable: false, Source: ocmReceivedProtocols, PrimaryKey: true, AutoIncrement: true},
		{Name: "ocm_received_share_id", Type: sql.Int64, Nullable: false, Source: ocmReceivedProtocols},
		{Name: "type", Type: sql.Int8, Nullable: false, Source: ocmReceivedProtocols},
	}), &fkAccessMethods)
	tables[ocmReceivedProtocols] = protocols

	// ocm_protocol_webdav table
	var kfProtocols memory.ForeignKeyCollection
	kfProtocols.AddFK(sql.ForeignKeyConstraint{
		Columns:       []string{"ocm_protocol_id"},
		ParentTable:   ocmReceivedProtocols,
		ParentColumns: []string{"id"},
		OnDelete:      sql.ForeignKeyReferentialAction_Cascade,
	})
	webdav := memory.NewTable(ocmProtWebDAVTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "ocm_protocol_id", Type: sql.Int64, Source: ocmProtWebDAVTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "uri", Type: sql.Text, Source: ocmProtWebDAVTable, Nullable: false},
		{Name: "shared_secret", Type: sql.Text, Source: ocmProtWebDAVTable, Nullable: false},
		{Name: "permissions", Type: sql.Int64, Source: ocmProtWebDAVTable, Nullable: false},
	}), &kfProtocols)
	tables[ocmProtWebDAVTable] = webdav

	// ocm_protocol_webapp table
	webapp := memory.NewTable(ocmProtWebappTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "ocm_protocol_id", Type: sql.Int64, Source: ocmProtWebappTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "uri_template", Type: sql.Text, Source: ocmProtWebappTable, Nullable: false},
		{Name: "view_mode", Type: sql.Int8, Nullable: false, Source: ocmProtWebappTable},
	}), &kfProtocols)
	tables[ocmProtWebappTable] = webapp

	// ocm_protocol_webapp table
	transfer := memory.NewTable(ocmProtTransferTable, sql.NewPrimaryKeySchema(sql.Schema{
		{Name: "ocm_protocol_id", Type: sql.Int64, Source: ocmProtTransferTable, PrimaryKey: true, AutoIncrement: true},
		{Name: "source_uri", Type: sql.Text, Source: ocmProtTransferTable, Nullable: false},
		{Name: "shared_secret", Type: sql.Text, Source: ocmProtTransferTable, Nullable: false},
		{Name: "size", Type: sql.Int64, Source: ocmProtTransferTable, Nullable: false},
	}), &kfProtocols)
	tables[ocmProtTransferTable] = transfer

	// init data
	for _, share := range initData {
		var expiration uint64
		if share.Expiration != nil {
			expiration = share.Expiration.Seconds
		}

		must(tableShares.Insert(ctx, sql.NewRow(mustInt(share.Id.OpaqueId), share.Name, share.RemoteShareId, int8(convertFromCS3ResourceType(share.ResourceType)), share.Grantee.GetUserId().OpaqueId, fmt.Sprintf("%s@%s", share.Owner.OpaqueId, share.Owner.Idp), fmt.Sprintf("%s@%s", share.Creator.OpaqueId, share.Creator.Idp), share.Ctime.Seconds, share.Mtime.Seconds, expiration, int8(convertFromCS3OCMShareType(share.ShareType)), int8(convertFromCS3OCMShareState(share.State)))))

		for _, p := range share.Protocols {
			i := id()
			switch prot := p.Term.(type) {
			case *ocm.Protocol_WebdavOptions:
				must(protocols.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(WebDAVProtocol))))
				must(webdav.Insert(ctx, sql.NewRow(i, prot.WebdavOptions.Uri, prot.WebdavOptions.SharedSecret, int64(conversions.RoleFromResourcePermissions(prot.WebdavOptions.Permissions.Permissions).OCSPermissions()))))
			case *ocm.Protocol_WebappOptions:
				must(protocols.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(WebappProtocol))))
				must(webapp.Insert(ctx, sql.NewRow(i, prot.WebappOptions.UriTemplate, int8(prot.WebappOptions.ViewMode))))
			case *ocm.Protocol_TransferOptions:
				must(protocols.Insert(ctx, sql.NewRow(i, mustInt(share.Id.OpaqueId), int8(TransferProtocol))))
				must(transfer.Insert(ctx, sql.NewRow(i, prot.TransferOptions.SourceUri, prot.TransferOptions.SharedSecret, int64(prot.TransferOptions.Size))))
			}
		}
	}

	return tables
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustInt(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return i
}

func TestGetShare(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.Share
		query       *ocm.ShareReference
		user        *userpb.User
		expected    *ocm.Share
		err         error
	}{
		{
			description: "empty list",
			shares:      []*ocm.Share{},
			query:       &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "non-existing-id"}}},
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    nil,
			err:         share.ErrShareNotFound,
		},
		{
			description: "query by id",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			expected: &ocm.Share{
				Id:            &ocm.ShareId{OpaqueId: "1"},
				ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
				Name:          "file-name",
				Token:         "qwerty",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:         &userpb.UserId{OpaqueId: "einstein"},
				Creator:       &userpb.UserId{OpaqueId: "einstein"},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				Expiration:    &typesv1beta1.Timestamp{},
				AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
			},
		},
		{
			description: "query by token",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Token{
					Token: "qwerty",
				},
			},
			expected: &ocm.Share{
				Id:            &ocm.ShareId{OpaqueId: "1"},
				ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
				Name:          "file-name",
				Token:         "qwerty",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:         &userpb.UserId{OpaqueId: "einstein"},
				Creator:       &userpb.UserId{OpaqueId: "einstein"},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				Expiration:    &typesv1beta1.Timestamp{},
				AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
			},
		},
		{
			description: "query by token - not found",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Token{
					Token: "not-existing-token",
				},
			},
			err: share.ErrShareNotFound,
		},
		{
			description: "query by key",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Key{
					Key: &ocm.ShareKey{
						Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
						Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			expected: &ocm.Share{
				Id:            &ocm.ShareId{OpaqueId: "1"},
				ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
				Name:          "file-name",
				Token:         "qwerty",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:         &userpb.UserId{OpaqueId: "einstein"},
				Creator:       &userpb.UserId{OpaqueId: "einstein"},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				Expiration:    &typesv1beta1.Timestamp{},
				AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
			},
		},
		{
			description: "query by key - not found",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Key{
					Key: &ocm.ShareKey{
						Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
						Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			err:  share.ErrShareNotFound,
		},
		{
			description: "query by id - different user",
			shares: []*ocm.Share{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					ResourceId:    &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:          "file-name",
					Token:         "qwerty",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions())},
				},
			},
			query: &ocm.ShareReference{
				Spec: &ocm.ShareReference_Key{
					Key: &ocm.ShareKey{
						Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "cernbox"},
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
						Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			err:  share.ErrShareNotFound,
		},
		{
			description: "all access methods",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			expected: &ocm.Share{
				Id:         &ocm.ShareId{OpaqueId: "1"},
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
				Name:       "file-name",
				Token:      "qwerty",
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				Creator:    &userpb.UserId{OpaqueId: "einstein"},
				Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:  ocm.ShareType_SHARE_TYPE_USER,
				Expiration: &typesv1beta1.Timestamp{},
				AccessMethods: []*ocm.AccessMethod{
					share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					share.NewTransferAccessMethod(),
				},
			},
		},
		{
			description: "owner gets the share create from an other user",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			expected: &ocm.Share{
				Id:         &ocm.ShareId{OpaqueId: "1"},
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
				Name:       "file-name",
				Token:      "qwerty",
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:      &userpb.UserId{OpaqueId: "marie"},
				Creator:    &userpb.UserId{OpaqueId: "einstein"},
				Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:  ocm.ShareType_SHARE_TYPE_USER,
				Expiration: &typesv1beta1.Timestamp{},
				AccessMethods: []*ocm.AccessMethod{
					share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					share.NewTransferAccessMethod(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createShareTables(ctx, tt.shares)
			_, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			got, err := r.GetShare(context.TODO(), tt.user, tt.query)
			if err != tt.err {
				t.Fatalf("not expected error getting share. got=%+v expected=%+v", err, tt.err)
			}

			if tt.err == nil {
				if !reflect.DeepEqual(got, tt.expected) {
					t.Fatalf("shares do not match. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
				}
			}
		})
	}
}

func TestListShares(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.Share
		filters     []*ocm.ListOCMSharesRequest_Filter
		user        *userpb.User
		expected    []*ocm.Share
	}{
		{
			description: "empty list",
			shares:      []*ocm.Share{},
			filters:     nil,
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    []*ocm.Share{},
		},
		{
			description: "share belong to the user",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
			},
			filters: nil,
			user:    &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			expected: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
			},
		},
		{
			description: "all shares belong to the user",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			filters: nil,
			user:    &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"}},
			expected: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Expiration: &typesv1beta1.Timestamp{},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
		},
		{
			description: "select share by user",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			filters: nil,
			user:    &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			expected: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "marie"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
		},
		{
			description: "filter by resource id",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id2"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			filters: []*ocm.ListOCMSharesRequest_Filter{
				{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
					Term: &ocm.ListOCMSharesRequest_Filter_ResourceId{
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id2"},
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			expected: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id2"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "marie"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
		},
		{
			description: "filter by resource id - empty result",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id2"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			filters: []*ocm.ListOCMSharesRequest_Filter{
				{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
					Term: &ocm.ListOCMSharesRequest_Filter_ResourceId{
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					},
				},
			},
			user:     &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			expected: []*ocm.Share{},
		},
		{
			description: "multiple filters",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "3"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id2"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
					Owner:      &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Creator:    &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			filters: []*ocm.ListOCMSharesRequest_Filter{
				{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
					Term: &ocm.ListOCMSharesRequest_Filter_ResourceId{
						ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					},
				},
				{
					Type: ocm.ListOCMSharesRequest_Filter_TYPE_OWNER,
					Term: &ocm.ListOCMSharesRequest_Filter_Owner{
						Owner: &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}},
			expected: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "1"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
						share.NewTransferAccessMethod(),
					},
				},
				{
					Id:         &ocm.ShareId{OpaqueId: "2"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					Expiration: &typesv1beta1.Timestamp{},
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createShareTables(ctx, tt.shares)
			_, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			got, err := r.ListShares(context.TODO(), tt.user, tt.filters)
			if err != nil {
				t.Fatalf("not expected error while listing shares: %+v", err)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("list of shares do not match. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		})
	}
}

type storeShareExpected struct {
	shares        []sql.Row
	accessmethods []sql.Row
	webdav        []sql.Row
	webapp        []sql.Row
}

func checkRows(ctx *sql.Context, engine *sqle.Engine, rows []sql.Row, table string, t *testing.T) {
	_, _, err := engine.Query(ctx, "USE "+dbName)
	if err != nil {
		t.Fatalf("got unexpected error: %+v", err)
	}

	_, iter, err := engine.Query(ctx, "SELECT * FROM "+table)
	if err != nil {
		t.Fatalf("got unexpected error: %+v", err)
	}

	gotRows := []sql.Row{}

	for {
		row, err := iter.Next(ctx)
		if err != nil {
			break
		}
		gotRows = append(gotRows, row)
	}

	if !reflect.DeepEqual(gotRows, rows) {
		t.Fatalf("rows are not equal. got=%+v expected=%+v", render.AsCode(gotRows), render.AsCode(rows))
	}
}

func checkShares(ctx *sql.Context, engine *sqle.Engine, exp storeShareExpected, t *testing.T) {
	checkRows(ctx, engine, exp.shares, ocmShareTable, t)
	checkRows(ctx, engine, exp.accessmethods, ocmAccessMethodTable, t)
	checkRows(ctx, engine, exp.webdav, ocmAMWebDAVTable, t)
	checkRows(ctx, engine, exp.webapp, ocmAMWebappTable, t)
}

func TestStoreShare(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.Share
		toStore     *ocm.Share
		err         error
		expected    storeShareExpected
	}{
		{
			description: "empty table",
			shares:      []*ocm.Share{},
			toStore: &ocm.Share{
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
				Name:       "file-name",
				Token:      "qwerty",
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				Creator:    &userpb.UserId{OpaqueId: "marie"},
				Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:  ocm.ShareType_SHARE_TYPE_USER,
				AccessMethods: []*ocm.AccessMethod{
					share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
				},
			},
			expected: storeShareExpected{
				shares: []sql.Row{{int64(1), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1670859468), uint64(1670859468), nil, int8(0)}},
				accessmethods: []sql.Row{
					{int64(1), int64(1), int8(0)},
					{int64(2), int64(1), int8(1)},
				},
				webdav: []sql.Row{{int64(1), int64(1)}},
				webapp: []sql.Row{{int64(2), int8(2)}},
			},
		},
		{
			description: "non empty table",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			toStore: &ocm.Share{
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "other-resource"},
				Name:       "file-name",
				Token:      "qwerty",
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				Creator:    &userpb.UserId{OpaqueId: "marie"},
				Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:  ocm.ShareType_SHARE_TYPE_USER,
				AccessMethods: []*ocm.AccessMethod{
					share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
				},
			},
			expected: storeShareExpected{
				shares: []sql.Row{
					{int64(10), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1670859468), uint64(1670859468), uint64(0), int8(0)},
					{int64(11), "qwerty", "storage", "other-resource", "file-name", "richard@cesnet", "einstein", "marie", uint64(1670859468), uint64(1670859468), nil, int8(0)},
				},
				accessmethods: []sql.Row{
					{int64(1), int64(10), int8(0)},
					{int64(2), int64(11), int8(0)},
				},
				webdav: []sql.Row{
					{int64(1), int64(1)},
					{int64(2), int64(1)},
				},
				webapp: []sql.Row{},
			},
		},
		{
			description: "share already exists",
			shares: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
					},
				},
			},
			toStore: &ocm.Share{
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
				Name:       "file-name",
				Token:      "qwerty",
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				Creator:    &userpb.UserId{OpaqueId: "marie"},
				Ctime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:      &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:  ocm.ShareType_SHARE_TYPE_USER,
				AccessMethods: []*ocm.AccessMethod{
					share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
				},
			},
			err: share.ErrShareAlreadyExisting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createShareTables(ctx, tt.shares)
			engine, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			_, err = r.StoreShare(context.TODO(), tt.toStore)
			if err != tt.err {
				t.Fatalf("not expected error getting share. got=%+v expected=%+v", err, tt.err)
			}

			if tt.err == nil {
				checkShares(ctx, engine, tt.expected, t)
			}
		})
	}
}

func TestUpdateShare(t *testing.T) {
	fixedTime := time.Date(2023, time.December, 12, 12, 12, 0, 0, time.UTC)

	tests := []struct {
		description string
		init        []*ocm.Share
		user        *userpb.User
		ref         *ocm.ShareReference
		fields      []*ocm.UpdateOCMShareRequest_UpdateField
		err         error
		expected    storeShareExpected
	}{
		{
			description: "update only expiration - by id",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user:   &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref:    &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "10"}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{Expiration: &typesv1beta1.Timestamp{Seconds: uint64(fixedTime.Unix())}}}},
			expected: storeShareExpected{
				shares: []sql.Row{{int64(10), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1686061921), uint64(fixedTime.Unix()), uint64(fixedTime.Unix()), int8(0)}},
				accessmethods: []sql.Row{
					{int64(1), int64(10), int8(0)},
					{int64(2), int64(10), int8(1)},
				},
				webdav: []sql.Row{{int64(1), int64(1)}},
				webapp: []sql.Row{{int64(2), int8(2)}},
			},
		},
		{
			description: "update access methods - by id",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref:  &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "10"}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
				},
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					},
				},
			},
			expected: storeShareExpected{
				shares: []sql.Row{{int64(10), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1686061921), uint64(fixedTime.Unix()), uint64(0), int8(0)}},
				accessmethods: []sql.Row{
					{int64(1), int64(10), int8(0)},
					{int64(2), int64(10), int8(1)},
				},
				webdav: []sql.Row{{int64(1), int64(15)}},
				webapp: []sql.Row{{int64(2), int8(3)}},
			},
		},
		{
			description: "update only expiration - by key",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
			}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{Expiration: &typesv1beta1.Timestamp{Seconds: uint64(fixedTime.Unix())}}}},
			expected: storeShareExpected{
				shares: []sql.Row{{int64(10), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1686061921), uint64(fixedTime.Unix()), uint64(fixedTime.Unix()), int8(0)}},
				accessmethods: []sql.Row{
					{int64(1), int64(10), int8(0)},
					{int64(2), int64(10), int8(1)},
				},
				webdav: []sql.Row{{int64(1), int64(1)}},
				webapp: []sql.Row{{int64(2), int8(2)}},
			},
		},
		{
			description: "update access methods - by key",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
				Owner:      &userpb.UserId{OpaqueId: "einstein"},
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
			}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
				},
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					},
				},
			},
			expected: storeShareExpected{
				shares: []sql.Row{{int64(10), "qwerty", "storage", "resource-id1", "file-name", "richard@cesnet", "einstein", "marie", uint64(1686061921), uint64(fixedTime.Unix()), uint64(0), int8(0)}},
				accessmethods: []sql.Row{
					{int64(1), int64(10), int8(0)},
					{int64(2), int64(10), int8(1)},
				},
				webdav: []sql.Row{{int64(1), int64(15)}},
				webapp: []sql.Row{{int64(2), int8(3)}},
			},
		},
		{
			description: "update only expiration - id not exists",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user:   &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref:    &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "not-existing-id"}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{Expiration: &typesv1beta1.Timestamp{Seconds: uint64(fixedTime.Unix())}}}},
			err:    share.ErrShareNotFound,
		},
		{
			description: "update access methods - key not exists",
			init: []*ocm.Share{
				{
					Id:         &ocm.ShareId{OpaqueId: "10"},
					ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
					Name:       "file-name",
					Token:      "qwerty",
					Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
					Owner:      &userpb.UserId{OpaqueId: "einstein"},
					Creator:    &userpb.UserId{OpaqueId: "marie"},
					Ctime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					Mtime:      &typesv1beta1.Timestamp{Seconds: 1686061921},
					ShareType:  ocm.ShareType_SHARE_TYPE_USER,
					AccessMethods: []*ocm.AccessMethod{
						share.NewWebDavAccessMethod(conversions.NewViewerRole().CS3ResourcePermissions()),
						share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}},
			ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
				Owner:      &userpb.UserId{OpaqueId: "non-existing-user"},
				ResourceId: &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "resource-id1"},
				Grantee:    &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED}}},
			}}},
			fields: []*ocm.UpdateOCMShareRequest_UpdateField{
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
					},
				},
				{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					},
				},
			},
			err: share.ErrShareNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createShareTables(ctx, tt.init)
			engine, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := NewFromConfig(
				&config{
					DBUsername: "root",
					DBPassword: "",
					DBAddress:  fmt.Sprintf("%s:%d", address, port),
					DBName:     dbName,
					now:        func() time.Time { return fixedTime },
				},
			)

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			_, err = r.UpdateShare(context.TODO(), tt.user, tt.ref, tt.fields...)
			if err != tt.err {
				t.Fatalf("not expected error updating share. got=%+v expected=%+v", err, tt.err)
			}

			if tt.err == nil {
				checkShares(ctx, engine, tt.expected, t)
			}
		})
	}
}

func TestGetReceivedShare(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.ReceivedShare
		query       *ocm.ShareReference
		user        *userpb.User
		expected    *ocm.ReceivedShare
		err         error
	}{
		{
			description: "empty list",
			shares:      []*ocm.ReceivedShare{},
			query:       &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "non-existing-id"}}},
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    nil,
			err:         share.ErrShareNotFound,
		},
		{
			description: "query by id",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
					},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}},
			expected: &ocm.ReceivedShare{
				Id:            &ocm.ShareId{OpaqueId: "1"},
				RemoteShareId: "1-remote",
				Name:          "file-name",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
				Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
				Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
				ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
				Expiration:    &typesv1beta1.Timestamp{},
				Protocols: []*ocm.Protocol{
					share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
						Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
					}),
				},
			},
		},
		{
			description: "query by id - different user",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
					},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "cernbox"}},
			err:   share.ErrShareNotFound,
		},
		{
			description: "all protocols",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
			},
			query: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: "1"}}},
			user:  &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}},
			expected: &ocm.ReceivedShare{
				Id:            &ocm.ShareId{OpaqueId: "1"},
				RemoteShareId: "1-remote",
				Name:          "file-name",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
				Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
				Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
				ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
				Expiration:    &typesv1beta1.Timestamp{},
				Protocols: []*ocm.Protocol{
					share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
						Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
					}),
					share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createReceivedShareTables(ctx, tt.shares)
			_, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			got, err := r.GetReceivedShare(context.TODO(), tt.user, tt.query)
			if err != tt.err {
				t.Fatalf("not expected error getting share. got=%+v expected=%+v", err, tt.err)
			}

			if tt.err == nil {
				if !reflect.DeepEqual(got, tt.expected) {
					t.Fatalf("shares do not match. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
				}
			}
		})
	}
}

func TestListReceviedShares(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.ReceivedShare
		user        *userpb.User
		expected    []*ocm.ReceivedShare
	}{
		{
			description: "empty list",
			shares:      []*ocm.ReceivedShare{},
			user:        &userpb.User{Id: &userpb.UserId{OpaqueId: "opaque", Idp: "idp"}},
			expected:    []*ocm.ReceivedShare{},
		},
		{
			description: "share belong to user",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}},
			expected: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Expiration:    &typesv1beta1.Timestamp{},
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
			},
		},
		{
			description: "all shares belong to user",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
				{
					Id:            &ocm.ShareId{OpaqueId: "2"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "richard"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "richard"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Protocols: []*ocm.Protocol{
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/54321", appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}},
			expected: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
					Expiration:    &typesv1beta1.Timestamp{},
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
				{
					Id:            &ocm.ShareId{OpaqueId: "2"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "richard", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Expiration:    &typesv1beta1.Timestamp{},
					Protocols: []*ocm.Protocol{
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/54321", appprovider.ViewMode_VIEW_MODE_READ_ONLY),
					},
				},
			},
		},
		{
			description: "select share by user",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
				{
					Id:            &ocm.ShareId{OpaqueId: "2"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "einstein"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "richard"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "richard"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Protocols: []*ocm.Protocol{
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/54321", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
					},
				},
			},
			user: &userpb.User{Id: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}},
			expected: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_FEDERATED},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
					Expiration:    &typesv1beta1.Timestamp{},
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
						share.NewWebappProtocol("https://cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
						share.NewTransferProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", 10),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createReceivedShareTables(ctx, tt.shares)
			_, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			got, err := r.ListReceivedShares(context.TODO(), tt.user)
			if err != nil {
				t.Fatalf("not expected error while listing received shares share repository driver: %+v", err)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("shares do not match. got=%+v expected=%+v", render.AsCode(got), render.AsCode(tt.expected))
			}
		})
	}
}

type storeReceivedShareExpected struct {
	shares    []sql.Row
	protocols []sql.Row
	webdav    []sql.Row
	webapp    []sql.Row
	transfer  []sql.Row
}

func checkReceivedShares(ctx *sql.Context, engine *sqle.Engine, exp storeReceivedShareExpected, t *testing.T) {
	checkRows(ctx, engine, exp.shares, ocmReceivedShareTable, t)
	checkRows(ctx, engine, exp.protocols, ocmReceivedProtocols, t)
	checkRows(ctx, engine, exp.webdav, ocmProtWebDAVTable, t)
	checkRows(ctx, engine, exp.webapp, ocmProtWebappTable, t)
	checkRows(ctx, engine, exp.transfer, ocmProtTransferTable, t)
}

func TestStoreReceivedShare(t *testing.T) {
	tests := []struct {
		description string
		shares      []*ocm.ReceivedShare
		toStore     *ocm.ReceivedShare
		err         error
		expected    storeReceivedShareExpected
	}{
		{
			description: "empty table",
			shares:      []*ocm.ReceivedShare{},
			toStore: &ocm.ReceivedShare{
				RemoteShareId: "1-remote",
				Name:          "file-name",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
				Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
				Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
				ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
				Protocols: []*ocm.Protocol{
					share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
						Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
					}),
				},
			},
			expected: storeReceivedShareExpected{
				shares:    []sql.Row{{int64(1), "file-name", "1-remote", int8(1), "marie", "einstein@cernbox", "einstein@cernbox", uint64(1670859468), uint64(1670859468), nil, int8(ShareTypeUser), int8(ShareStateAccepted)}},
				protocols: []sql.Row{{int64(1), int64(1), int8(0)}},
				webdav:    []sql.Row{{int64(1), "webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", int64(15)}},
				webapp:    []sql.Row{},
				transfer:  []sql.Row{},
			},
		},
		{
			description: "non empty table",
			shares: []*ocm.ReceivedShare{
				{
					Id:            &ocm.ShareId{OpaqueId: "1"},
					RemoteShareId: "1-remote",
					Name:          "file-name",
					Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "marie"}}},
					Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "einstein"},
					Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
					ShareType:     ocm.ShareType_SHARE_TYPE_USER,
					State:         ocm.ShareState_SHARE_STATE_ACCEPTED,
					ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_FILE,
					Protocols: []*ocm.Protocol{
						share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
							Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
						}),
					},
				},
			},
			toStore: &ocm.ReceivedShare{
				RemoteShareId: "2-remote",
				Name:          "file-name",
				Grantee:       &providerv1beta1.Grantee{Type: providerv1beta1.GranteeType_GRANTEE_TYPE_USER, Id: &providerv1beta1.Grantee_UserId{UserId: &userpb.UserId{Idp: "cesnet", OpaqueId: "richard"}}},
				Owner:         &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
				Creator:       &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"},
				Ctime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				Mtime:         &typesv1beta1.Timestamp{Seconds: 1670859468},
				ShareType:     ocm.ShareType_SHARE_TYPE_USER,
				State:         ocm.ShareState_SHARE_STATE_PENDING,
				ResourceType:  providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
				Protocols: []*ocm.Protocol{
					share.NewWebDAVProtocol("webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", &ocm.SharePermissions{
						Permissions: conversions.NewEditorRole().CS3ResourcePermissions(),
					}),
					share.NewTransferProtocol("https://transfer.cernbox.cern.ch/ocm/1234", "secret", 100),
					share.NewWebappProtocol("https://app.cernbox.cern.ch/ocm/1234", appprovider.ViewMode_VIEW_MODE_READ_WRITE),
				},
			},
			expected: storeReceivedShareExpected{
				shares: []sql.Row{
					{int64(1), "file-name", "1-remote", int8(0), "marie", "einstein@cernbox", "einstein@cernbox", uint64(1670859468), uint64(1670859468), uint64(0), int8(ShareTypeUser), int8(ShareStateAccepted)},
					{int64(2), "file-name", "2-remote", int8(1), "richard", "marie@cernbox", "marie@cernbox", uint64(1670859468), uint64(1670859468), nil, int8(ShareTypeUser), int8(ShareStatePending)},
				},
				protocols: []sql.Row{
					{int64(1), int64(1), int8(WebDAVProtocol)},
					{int64(2), int64(2), int8(WebDAVProtocol)},
					{int64(3), int64(2), int8(TransferProtocol)},
					{int64(4), int64(2), int8(WebappProtocol)},
				},
				webdav: []sql.Row{
					{int64(1), "webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", int64(15)},
					{int64(2), "webdav+https//cernbox.cern.ch/dav/ocm/1", "secret", int64(15)},
				},
				webapp: []sql.Row{
					{int64(4), "https://app.cernbox.cern.ch/ocm/1234", int8(3)},
				},
				transfer: []sql.Row{
					{int64(3), "https://transfer.cernbox.cern.ch/ocm/1234", "secret", int64(100)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := sql.NewEmptyContext()
			tables := createReceivedShareTables(ctx, tt.shares)
			engine, port, cleanup := startDatabase(ctx, tables)
			t.Cleanup(cleanup)

			r, err := New(map[string]interface{}{
				"db_username": "root",
				"db_password": "",
				"db_address":  fmt.Sprintf("%s:%d", address, port),
				"db_name":     dbName,
			})

			if err != nil {
				t.Fatalf("not expected error while creating share repository driver: %+v", err)
			}

			_, err = r.StoreReceivedShare(context.TODO(), tt.toStore)
			if err != tt.err {
				t.Fatalf("not expected error getting share. got=%+v expected=%+v", err, tt.err)
			}

			if tt.err == nil {
				checkReceivedShares(ctx, engine, tt.expected, t)
			}
		})
	}
}
