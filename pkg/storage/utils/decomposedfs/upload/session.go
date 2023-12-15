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

// Session is the interface that OcisSession implements. By combining tus.Upload,
// storage.UploadSession and custom functions we can reuse the same struct throughout
// the whole upload lifecycle.
//
// Some functions that are only used by decomposedfs are not yet part of this interface.
// They might be added after more refactoring.
type Session interface {
	tusd.Upload
	storage.UploadSession
	Persist(ctx context.Context) error
	Node(ctx context.Context) (*node.Node, error)
	LockID() string
	Context(ctx context.Context) context.Context
	Cleanup(cleanNode, cleanBin, cleanInfo bool)
}

// OcisSession extends tus upload lifecycle with postprocessing steps.
type OcisSession struct {
	store OcisStore
	// for now, we keep the json files in the uploads folder
	info tusd.FileInfo
	// a context that is reinitialized with the executant and log from the session metadata
	ctx context.Context
}

func (m *OcisSession) Context(ctx context.Context) context.Context {
	ctx, _ = m.ContextWithLogger(ctx) // ignore the error
	return m.ContextWithExecutant(ctx)
}
func (m *OcisSession) ContextWithExecutant(ctx context.Context) context.Context {
	return ctxpkg.ContextSetUser(ctx, m.ExecutantUser())
}
func (m *OcisSession) ContextWithLogger(ctx context.Context) (context.Context, error) {
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

func (m *OcisSession) Purge(ctx context.Context) error {
	if err := os.Remove(sessionPath(m.store.root, m.info.ID)); err != nil {
		return err
	}
	if err := os.Remove(m.binPath()); err != nil {
		return err
	}
	return nil
}

func (m *OcisSession) TouchBin() error {
	file, err := os.OpenFile(m.binPath(), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return err
	}
	return file.Close()
}

func (m *OcisSession) Persist(ctx context.Context) error {
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

func (m *OcisSession) ToFileInfo() tusd.FileInfo {
	return m.info
}
func (m *OcisSession) ProviderID() string {
	return m.info.MetaData["providerID"]
}
func (m *OcisSession) SpaceID() string {
	return m.info.Storage["SpaceRoot"]
}
func (m *OcisSession) NodeID() string {
	return m.info.Storage["NodeId"]
}
func (m *OcisSession) NodeParentID() string {
	return m.info.Storage["NodeParentId"]
}
func (m *OcisSession) NodeExists() bool {
	return m.info.Storage["NodeExists"] == "true"
}
func (m *OcisSession) LockID() string {
	return m.info.MetaData["lockid"]
}
func (m *OcisSession) ETag() string {
	return m.info.MetaData["etag"]
}
func (m *OcisSession) HeaderIfMatch() string {
	return m.info.MetaData["if-match"]
}
func (m *OcisSession) HeaderIfNoneMatch() string {
	return m.info.MetaData["if-none-match"]
}
func (m *OcisSession) HeaderIfUnmodifiedSince() string {
	return m.info.MetaData["if-unmodified-since"]
}
func (m *OcisSession) Node(ctx context.Context) (*node.Node, error) {
	n, err := node.ReadNode(ctx, m.store.lu, m.SpaceID(), m.info.Storage["NodeId"], false, nil, true)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (m *OcisSession) ID() string {
	return m.info.ID
}
func (m *OcisSession) Filename() string {
	return m.info.Storage["NodeName"]
}

func (m *OcisSession) SetMetadata(key, value string) {
	m.info.MetaData[key] = value
}
func (m *OcisSession) SetStorageValue(key, value string) {
	m.info.Storage[key] = value
}
func (m *OcisSession) SetSize(size int64) {
	m.info.Size = size
}
func (m *OcisSession) SetSizeIsDeferred(value bool) {
	m.info.SizeIsDeferred = value
}

// TODO get rid of dir, whoever consumes the reference should be able to deal with a relative reference
func (m *OcisSession) Dir() string {
	return m.info.Storage["Dir"]
}

func (m *OcisSession) Size() int64 {
	return m.info.Size
}
func (m *OcisSession) SizeDiff() int64 {
	sizeDiff, _ := strconv.ParseInt(m.info.MetaData["sizeDiff"], 10, 64)
	return sizeDiff
}

func (m *OcisSession) ResourceID() provider.ResourceId {
	return provider.ResourceId{
		StorageId: m.info.MetaData["providerID"],
		SpaceId:   m.info.Storage["SpaceRoot"],
		OpaqueId:  m.info.Storage["NodeId"],
	}
}
func (m *OcisSession) Reference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: m.info.MetaData["providerID"],
			SpaceId:   m.info.Storage["SpaceRoot"],
			OpaqueId:  m.info.Storage["NodeId"],
		},
		// Path is not used
	}
}
func (m *OcisSession) Executant() userpb.UserId {
	return userpb.UserId{
		Type:     userpb.UserType(userpb.UserType_value[m.info.Storage["UserType"]]),
		Idp:      m.info.Storage["Idp"],
		OpaqueId: m.info.Storage["UserId"],
	}
}
func (m *OcisSession) ExecutantUser() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			Type:     userpb.UserType(userpb.UserType_value[m.info.Storage["UserType"]]),
			Idp:      m.info.Storage["Idp"],
			OpaqueId: m.info.Storage["UserId"],
		},
		Username: m.info.Storage["UserName"],
	}
}
func (m *OcisSession) SetExecutant(u *userpb.User) {
	m.info.Storage["Idp"] = u.GetId().GetIdp()
	m.info.Storage["UserId"] = u.GetId().GetOpaqueId()
	m.info.Storage["UserType"] = utils.UserTypeToString(u.GetId().Type)
	m.info.Storage["UserName"] = u.GetUsername()
}
func (m *OcisSession) Offset() int64 {
	return m.info.Offset
}
func (m *OcisSession) SpaceOwner() *userpb.UserId {
	return &userpb.UserId{
		// idp and type do not seem to be consumed and the node currently only stores the user id anyway
		OpaqueId: m.info.Storage["SpaceOwnerOrManager"],
	}

}
func (m *OcisSession) Expires() time.Time {
	var t time.Time
	if value, ok := m.info.MetaData["expires"]; ok {
		t, _ = utils.MTimeToTime(value)
	}
	return t
}
func (m *OcisSession) MTime() time.Time {
	var t time.Time
	if value, ok := m.info.MetaData["mtime"]; ok {
		t, _ = utils.MTimeToTime(value)
	}
	return t
}
func (m *OcisSession) IsProcessing() bool {
	return m.info.Storage["processing"] != "" // FIXME
}

// binPath returns the path to the file storing the binary data.
func (m *OcisSession) binPath() string {
	return filepath.Join(m.store.root, "uploads", m.info.ID)
}

// infoPath returns the path to the .info file storing the file's info.
func sessionPath(root, id string) string {
	return filepath.Join(root, "uploads", id+".info")
}
