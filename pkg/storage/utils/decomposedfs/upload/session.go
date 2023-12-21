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
}

func (s *OcisSession) Context(ctx context.Context) context.Context { // restore logger from file info
	log, _ := logger.FromConfig(&logger.LogConf{
		Output: "stderr", // TODO use config from decomposedfs
		Mode:   "json",   // TODO use config from decomposedfs
		Level:  s.info.Storage["LogLevel"],
	})
	sub := log.With().Int("pid", os.Getpid()).Logger()
	ctx = appctx.WithLogger(ctx, &sub)
	ctx = ctxpkg.ContextSetLockID(ctx, s.LockID())
	return ctxpkg.ContextSetUser(ctx, s.ExecutantUser())
}

func (s *OcisSession) Purge(ctx context.Context) error {
	if err := os.Remove(sessionPath(s.store.root, s.info.ID)); err != nil {
		return err
	}
	if err := os.Remove(s.binPath()); err != nil {
		return err
	}
	return nil
}

func (s *OcisSession) TouchBin() error {
	file, err := os.OpenFile(s.binPath(), os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return err
	}
	return file.Close()
}

func (s *OcisSession) Persist(ctx context.Context) error {
	uploadPath := sessionPath(s.store.root, s.info.ID)
	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0700); err != nil {
		return err
	}

	var d []byte
	d, err := json.Marshal(s.info)
	if err != nil {
		return err
	}

	return os.WriteFile(uploadPath, d, 0600)
}

func (s *OcisSession) ToFileInfo() tusd.FileInfo {
	return s.info
}
func (s *OcisSession) ProviderID() string {
	return s.info.MetaData["providerID"]
}
func (s *OcisSession) SpaceID() string {
	return s.info.Storage["SpaceRoot"]
}
func (s *OcisSession) NodeID() string {
	return s.info.Storage["NodeId"]
}
func (s *OcisSession) NodeParentID() string {
	return s.info.Storage["NodeParentId"]
}
func (s *OcisSession) NodeExists() bool {
	return s.info.Storage["NodeExists"] == "true"
}
func (s *OcisSession) LockID() string {
	return s.info.MetaData["lockid"]
}
func (s *OcisSession) ETag() string {
	return s.info.MetaData["etag"]
}
func (s *OcisSession) HeaderIfMatch() string {
	return s.info.MetaData["if-match"]
}
func (s *OcisSession) HeaderIfNoneMatch() string {
	return s.info.MetaData["if-none-match"]
}
func (s *OcisSession) HeaderIfUnmodifiedSince() string {
	return s.info.MetaData["if-unmodified-since"]
}
func (s *OcisSession) Node(ctx context.Context) (*node.Node, error) {
	n, err := node.ReadNode(ctx, s.store.lu, s.SpaceID(), s.info.Storage["NodeId"], false, nil, true)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (s *OcisSession) ID() string {
	return s.info.ID
}
func (s *OcisSession) Filename() string {
	return s.info.Storage["NodeName"]
}
func (s *OcisSession) Chunk() string {
	return s.info.Storage["Chunk"]
}
func (s *OcisSession) SetMetadata(key, value string) {
	s.info.MetaData[key] = value
}
func (s *OcisSession) SetStorageValue(key, value string) {
	s.info.Storage[key] = value
}
func (s *OcisSession) SetSize(size int64) {
	s.info.Size = size
}
func (s *OcisSession) SetSizeIsDeferred(value bool) {
	s.info.SizeIsDeferred = value
}

// TODO get rid of dir, whoever consumes the reference should be able to deal with a relative reference
// Dir is only used to:
// * fill the Path property when emitting the UploadReady event after postprocessing finished. I wonder why the UploadReady contains a finished flag ... maybe multiple distinct events would make more sense.
// * build the reference that is passed to the FileUploaded event in the UploadFinishedFunc callback passed to the Upload call used for simple datatx put requests
// * AFAICT only search and audir consume the path.
//   - search needs to index from the root anyway. and it only needs the most recent path to put it in the index
//   - audit on the other hand needs to log events with the path at the state of the event ... so it does need the full path.
//     I think we can safely read determine the path later, right before emitting the event. and maybe make it configurable, because only audit needs it, anyway.
func (s *OcisSession) Dir() string {
	return s.info.Storage["Dir"]
}

func (s *OcisSession) Size() int64 {
	return s.info.Size
}
func (s *OcisSession) SizeDiff() int64 {
	sizeDiff, _ := strconv.ParseInt(s.info.MetaData["sizeDiff"], 10, 64)
	return sizeDiff
}

func (s *OcisSession) ResourceID() provider.ResourceId {
	return provider.ResourceId{
		StorageId: s.info.MetaData["providerID"],
		SpaceId:   s.info.Storage["SpaceRoot"],
		OpaqueId:  s.info.Storage["NodeId"],
	}
}
func (s *OcisSession) Reference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: s.info.MetaData["providerID"],
			SpaceId:   s.info.Storage["SpaceRoot"],
			OpaqueId:  s.info.Storage["NodeId"],
		},
		// Path is not used
	}
}
func (s *OcisSession) Executant() userpb.UserId {
	return userpb.UserId{
		Type:     userpb.UserType(userpb.UserType_value[s.info.Storage["UserType"]]),
		Idp:      s.info.Storage["Idp"],
		OpaqueId: s.info.Storage["UserId"],
	}
}
func (s *OcisSession) ExecutantUser() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			Type:     userpb.UserType(userpb.UserType_value[s.info.Storage["UserType"]]),
			Idp:      s.info.Storage["Idp"],
			OpaqueId: s.info.Storage["UserId"],
		},
		Username: s.info.Storage["UserName"],
	}
}
func (s *OcisSession) SetExecutant(u *userpb.User) {
	s.info.Storage["Idp"] = u.GetId().GetIdp()
	s.info.Storage["UserId"] = u.GetId().GetOpaqueId()
	s.info.Storage["UserType"] = utils.UserTypeToString(u.GetId().Type)
	s.info.Storage["UserName"] = u.GetUsername()
}
func (s *OcisSession) Offset() int64 {
	return s.info.Offset
}
func (s *OcisSession) SpaceOwner() *userpb.UserId {
	return &userpb.UserId{
		// idp and type do not seem to be consumed and the node currently only stores the user id anyway
		OpaqueId: s.info.Storage["SpaceOwnerOrManager"],
	}

}
func (s *OcisSession) Expires() time.Time {
	var t time.Time
	if value, ok := s.info.MetaData["expires"]; ok {
		t, _ = utils.MTimeToTime(value)
	}
	return t
}
func (s *OcisSession) MTime() time.Time {
	var t time.Time
	if value, ok := s.info.MetaData["mtime"]; ok {
		t, _ = utils.MTimeToTime(value)
	}
	return t
}
func (s *OcisSession) IsProcessing() bool {
	return s.info.Storage["processing"] != "" // FIXME
}

// binPath returns the path to the file storing the binary data.
func (s *OcisSession) binPath() string {
	return filepath.Join(s.store.root, "uploads", s.info.ID)
}

// infoPath returns the path to the .info file storing the file's info.
func sessionPath(root, id string) string {
	return filepath.Join(root, "uploads", id+".info")
}
