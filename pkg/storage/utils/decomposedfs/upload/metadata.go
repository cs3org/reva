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

package upload

import (
	"context"
	"os"
	"path/filepath"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/lookup"
	"github.com/shamaton/msgpack/v2"
)

type Metadata struct {
	Filename                string
	SpaceRoot               string
	SpaceOwnerOrManager     string
	ProviderID              string
	RevisionTime            string
	NodeId                  string
	NodeParentId            string
	ExecutantIdp            string
	ExecutantId             string
	ExecutantType           string
	ExecutantUserName       string
	LogLevel                string
	Checksum                string
	Chunk                   string
	Dir                     string
	LockID                  string
	HeaderIfMatch           string
	HeaderIfNoneMatch       string
	HeaderIfUnmodifiedSince string
	Expires                 string
}

// WriteMetadata will create a metadata file to keep track of an upload
func WriteMetadata(ctx context.Context, lu *lookup.Lookup, spaceID, nodeID, uploadID string, metadata Metadata) error {
	_, span := tracer.Start(ctx, "WriteMetadata")
	defer span.End()

	uploadPath := lu.UploadPath(uploadID)

	// create folder structure (if needed)
	if err := os.MkdirAll(filepath.Dir(uploadPath), 0700); err != nil {
		return err
	}

	var d []byte
	d, err := msgpack.Marshal(metadata)
	if err != nil {
		return err
	}

	_, subspan := tracer.Start(ctx, "os.Writefile")
	err = os.WriteFile(uploadPath, d, 0600)
	subspan.End()
	if err != nil {
		return err
	}

	return nil
}
func ReadMetadata(ctx context.Context, lu *lookup.Lookup, uploadID string) (Metadata, error) {
	_, span := tracer.Start(ctx, "ReadMetadata")
	defer span.End()

	uploadPath := lu.UploadPath(uploadID)

	_, subspan := tracer.Start(ctx, "os.ReadFile")
	msgBytes, err := os.ReadFile(uploadPath)
	subspan.End()
	if err != nil {
		return Metadata{}, err
	}

	metadata := Metadata{}
	if len(msgBytes) > 0 {
		err = msgpack.Unmarshal(msgBytes, &metadata)
		if err != nil {
			return Metadata{}, err
		}
	}
	return metadata, nil
}

func (m Metadata) GetResourceID() provider.ResourceId {
	return provider.ResourceId{
		StorageId: m.ProviderID,
		SpaceId:   m.SpaceRoot,
		OpaqueId:  m.NodeId,
	}
}
func (m Metadata) GetReference() provider.Reference {
	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: m.ProviderID,
			SpaceId:   m.SpaceRoot,
			OpaqueId:  m.NodeId,
		},
		// Parh is not used
	}
}
func (m Metadata) GetExecutantID() userpb.UserId {
	return userpb.UserId{
		Type:     userpb.UserType(userpb.UserType_value[m.ExecutantType]),
		Idp:      m.ExecutantIdp,
		OpaqueId: m.ExecutantId,
	}
}
func (m Metadata) GetSpaceOwner() userpb.UserId {
	return userpb.UserId{
		OpaqueId: m.SpaceOwnerOrManager,
		// TODO the rest?
	}

}
func (m Metadata) GetExpires() string {
	return m.Expires // TODO use time?
}
