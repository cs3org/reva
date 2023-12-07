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

package tus

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/shamaton/msgpack/v2"
	tusd "github.com/tus/tusd/pkg/handler"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type Session struct {
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
}

func NewSession(ctx context.Context, root string) Session {
	return Session{
		root:     root,
		MetaData: tusd.MetaData{},
	}
}

func ReadSession(ctx context.Context, root, id string) (Session, error) {
	uploadPath := sessionPath(root, id)

	msgBytes, err := os.ReadFile(uploadPath)
	if err != nil {
		return Session{}, err
	}

	metadata := Session{}
	if len(msgBytes) > 0 {
		err = msgpack.Unmarshal(msgBytes, &metadata)
		if err != nil {
			return Session{}, err
		}
	}
	metadata.root = root
	return metadata, nil
}

func (m Session) Purge(ctx context.Context) error {
	return os.Remove(sessionPath(m.root, m.ID))
}

func (m Session) Persist(ctx context.Context) error {
	uploadPath := sessionPath(m.root, m.ID)
	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0700); err != nil {
		return err
	}

	var d []byte
	d, err := msgpack.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(uploadPath, d, 0600)
}

func (m Session) ToFileInfo() tusd.FileInfo {
	return tusd.FileInfo{
		ID:             m.ID,
		MetaData:       m.MetaData,
		Size:           m.Size,
		SizeIsDeferred: m.SizeIsDeferred,
		IsPartial:      false,
		IsFinal:        false,
		Offset:         m.Offset,
	}
}

func (m Session) GetID() string {
	return m.ID
}
func (m Session) GetFilename() string {
	return m.Filename
}

// TODO use uint64? use SizeDeferred flag is in tus? cleaner then int64 and a negative value
func (m Session) GetSize() int64 {
	return m.BlobSize
}
func (m Session) GetResourceID() provider.ResourceId {
	return provider.ResourceId{
		StorageId: m.ProviderID,
		SpaceId:   m.SpaceRoot,
		OpaqueId:  m.NodeID,
	}
}
func (m Session) GetReference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: m.ProviderID,
			SpaceId:   m.SpaceRoot,
			OpaqueId:  m.NodeID,
		},
		// Path is not used
	}
}
func (m Session) GetExecutantID() userpb.UserId {
	return userpb.UserId{
		Type:     userpb.UserType(userpb.UserType_value[m.ExecutantType]),
		Idp:      m.ExecutantIdp,
		OpaqueId: m.ExecutantID,
	}
}
func (m Session) GetSpaceOwner() userpb.UserId {
	return userpb.UserId{
		// idp and type do not seem to be consumed and the node currently only stores the user id anyway
		OpaqueId: m.SpaceOwnerOrManager,
	}

}
func (m Session) GetExpires() time.Time {
	return m.Expires
}

func sessionPath(root, id string) string {
	return filepath.Join(root, "uploads", id+".mpk")
}
