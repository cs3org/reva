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

package upload

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tusd "github.com/tus/tusd/pkg/handler"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/logger"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/node"
	"github.com/cs3org/reva/v2/pkg/utils"
)

type UploadSession interface {
	tusd.Upload
	storage.UploadSession
	Persist(ctx context.Context) error
	Node(ctx context.Context) (*node.Node, error)
	LockID() string
	Context(ctx context.Context) context.Context
	Cleanup(cleanNode, cleanBin, cleanInfo bool)
}

type Session struct {
	store ocisstore
	// for now, we keep the json files in the uploads folder
	info tusd.FileInfo
	// a context that is reinitialized with the executant and log from the session metadata
	ctx context.Context
	/*
		ID             string
		MetaData       tusd.MetaData
		Size           int64
		SizeIsDeferred bool
		Offset         int64
		Storage        map[string]string

		Filename            string
		SpaceRoot           string
		SpaceOwnerOrManager string
		ProviderID          string
		MTime               string

		NodeID       string
		NodeParentID string
		NodeExists   bool

		ExecutantIdp      string
		ExecutantID       string
		ExecutantType     string
		ExecutantUserName string
		LogLevel          string
		Checksum          string
		ChecksumSHA1      string
		ChecksumADLER32   string
		ChecksumMD5       string

		BlobID   string
		BlobSize int64

		// BinPath holds the path to the uploaded blob
		BinPath string

		// SizeDiff size difference between new and old file version
		SizeDiff int64

		// versionsPath will be empty if there was no file before
		VersionsPath string

		Chunk                   string
		Dir                     string
		LockID                  string
		HeaderIfMatch           string
		HeaderIfNoneMatch       string
		HeaderIfUnmodifiedSince string
		Expires                 time.Time

		root string
	*/
}

func (m *Session) Context(ctx context.Context) context.Context {
	ctx, _ = m.ContextWithLogger(ctx) // ignore the error
	return m.ContextWithExecutant(ctx)
}
func (m *Session) ContextWithExecutant(ctx context.Context) context.Context {
	return ctxpkg.ContextSetUser(ctx, m.ExecutantUser())
}
func (m *Session) ContextWithLogger(ctx context.Context) (context.Context, error) {
	// restore logger from file info
	log, err := logger.FromConfig(&logger.LogConf{
		Output: "stderr", // TODO use config from decomposedfs
		Mode:   "json",   // TODO use config from decomposedfs
		Level:  m.info.Storage["LogLevel"],
	})
	if err != nil {
		return ctx, err
	}
	sub := log.With().Int("pid", os.Getpid()).Logger()
	return appctx.WithLogger(ctx, &sub), nil
}

func (m *Session) Purge(ctx context.Context) error {
	if err := os.Remove(sessionPath(m.store.root, m.info.ID)); err != nil {
		return err
	}
	if err := os.Remove(m.binPath()); err != nil {
		return err
	}
	return nil
}

func (m *Session) TouchBin() error {
	file, err := os.OpenFile(m.binPath(), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return err
	}
	return file.Close()
}

func (m *Session) Persist(ctx context.Context) error {
	uploadPath := sessionPath(m.store.root, m.info.ID)
	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0700); err != nil {
		return err
	}

	var d []byte
	d, err := json.Marshal(m.info)
	if err != nil {
		return err
	}

	return os.WriteFile(uploadPath, d, 0600)
}

func (m *Session) ToFileInfo() tusd.FileInfo {
	return m.info
}
func (m *Session) ProviderID() string {
	return m.info.MetaData["providerID"]
}
func (m *Session) SpaceID() string {
	return m.info.Storage["SpaceRoot"]
}
func (m *Session) NodeID() string {
	return m.info.Storage["NodeId"]
}
func (m *Session) NodeParentID() string {
	return m.info.Storage["NodeParentId"]
}
func (m *Session) NodeExists() bool {
	return m.info.Storage["NodeExists"] == "true"
}
func (m *Session) LockID() string {
	return m.info.MetaData["lockid"]
}
func (m *Session) ETag() string {
	return m.info.MetaData["etag"]
}
func (m *Session) HeaderIfMatch() string {
	return m.info.MetaData["if-match"]
}
func (m *Session) HeaderIfNoneMatch() string {
	return m.info.MetaData["if-none-match"]
}
func (m *Session) HeaderIfUnmodifiedSince() string {
	return m.info.MetaData["if-unmodified-since"]
}
func (m *Session) Node(ctx context.Context) (*node.Node, error) {
	n, err := node.ReadNode(ctx, m.store.lu, m.SpaceID(), m.info.Storage["NodeId"], false, nil, true)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (m *Session) ID() string {
	return m.info.ID
}
func (m *Session) Filename() string {
	return m.info.Storage["NodeName"]
}

func (m *Session) SetMetadata(key, value string) {
	m.info.MetaData[key] = value
}
func (m *Session) SetStorageValue(key, value string) {
	m.info.Storage[key] = value
}
func (m *Session) SetSize(size int64) {
	m.info.Size = size
}
func (m *Session) SetSizeIsDeferred(value bool) {
	m.info.SizeIsDeferred = value
}

// TODO get rid of dir, whoever consumes the reference should be able to deal with a relative reference
func (m *Session) Dir() string {
	return m.info.Storage["Dir"]
}

// TODO use uint64? use SizeDeferred flag is in tus? cleaner then int64 and a negative value
func (m *Session) Size() int64 {
	return m.info.Size
}
func (m *Session) SizeDiff() int64 {
	sizeDiff, _ := strconv.ParseInt(m.info.MetaData["sizeDiff"], 10, 64)
	return sizeDiff
}

func (m *Session) ResourceID() provider.ResourceId {
	return provider.ResourceId{
		StorageId: m.info.MetaData["providerID"],
		SpaceId:   m.info.Storage["SpaceRoot"],
		OpaqueId:  m.info.Storage["NodeId"],
	}
}
func (m *Session) Reference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: m.info.MetaData["providerID"],
			SpaceId:   m.info.Storage["SpaceRoot"],
			OpaqueId:  m.info.Storage["NodeId"],
		},
		// Path is not used
	}
}
func (m *Session) Executant() userpb.UserId {
	return userpb.UserId{
		Type:     userpb.UserType(userpb.UserType_value[m.info.Storage["UserType"]]),
		Idp:      m.info.Storage["Idp"],
		OpaqueId: m.info.Storage["UserId"],
	}
}
func (m *Session) ExecutantUser() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			Type:     userpb.UserType(userpb.UserType_value[m.info.Storage["UserType"]]),
			Idp:      m.info.Storage["Idp"],
			OpaqueId: m.info.Storage["UserId"],
		},
		Username: m.info.Storage["UserName"],
	}
}
func (m *Session) SetExecutant(u *userpb.User) {
	m.info.Storage["Idp"] = u.GetId().GetIdp()
	m.info.Storage["UserId"] = u.GetId().GetOpaqueId()
	m.info.Storage["UserType"] = utils.UserTypeToString(u.GetId().Type)
	m.info.Storage["UserName"] = u.GetUsername()
}
func (m *Session) Offset() int64 {
	return m.info.Offset
}
func (m *Session) SpaceOwner() *userpb.UserId {
	return &userpb.UserId{
		// idp and type do not seem to be consumed and the node currently only stores the user id anyway
		OpaqueId: m.info.Storage["SpaceOwnerOrManager"],
	}

}
func (m *Session) Expires() time.Time {
	mt, _ := utils.MTimeToTime(m.info.MetaData["expires"])
	return mt
}
func (m *Session) MTime() time.Time {
	mt, _ := utils.MTimeToTime(m.info.MetaData["mtime"])
	return mt
}
func (m *Session) IsProcessing() bool {
	return m.info.Storage["processing"] != "" // FIXME
}

// binPath returns the path to the file storing the binary data.
func (m *Session) binPath() string {
	return filepath.Join(m.store.root, "uploads", m.info.ID)
}

// infoPath returns the path to the .info file storing the file's info.
func sessionPath(root, id string) string {
	return filepath.Join(root, "uploads", id+".info")
}
