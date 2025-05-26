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

package spaces

import (
	"encoding/base32"
	"errors"
	"fmt"
	"os"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// DecodeStorageSpaceID returns the components of the storage + space ID.
// This ID is expected to be in the format <storage_id>$base32(<path>).
func DecodeStorageSpaceID(raw string) (storageID, path string, ok bool) {
	// The input is expected to be in the form of <storage_id>$<base32(<path>)
	s := strings.SplitN(raw, "$", 2)
	if len(s) != 2 {
		return "", "", false
	}

	storageID = s[0]
	encodedPath := s[1]
	path, err := DecodeSpaceID(encodedPath)
	if err != nil {
		return "", "", false
	}
	return storageID, path, true
}

func EncodeSpaceID(spacePath string) string {
	return base32.StdEncoding.EncodeToString([]byte(spacePath))
}

func DecodeSpaceID(spaceId string) (string, error) {
	res, err := base32.StdEncoding.DecodeString(spaceId)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// Decode resourceID returns the components of the space ID.
// The resource ID is expected to be in the form of <storage_id>$<base32(<path>)!<item_id>.
func DecodeResourceID(raw string) (storageID, path, itemID string, ok bool) {
	// The input is expected to be in the form of <storage_id>$base32(<path>)!<item_id>
	s := strings.SplitN(raw, "+", 2)
	if len(s) != 2 {
		return "", "", "", false
	}
	itemID = s[1]
	storageID, path, ok = DecodeStorageSpaceID(s[0])
	return storageID, path, itemID, ok
}

// ParseResourceID converts the encoded resource id in a CS3API ResourceId.
func ParseResourceID(raw string) (*provider.ResourceId, bool) {
	storageID, path, itemID, ok := DecodeResourceID(raw)
	if !ok {
		return nil, false
	}
	return &provider.ResourceId{
		StorageId: storageID,
		SpaceId:   path,
		OpaqueId:  itemID,
	}, true
}

// EncodeResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
// If space_id or opaque_id is not set on the ResourceId,
// then this part will not be encoded
func EncodeResourceID(r *provider.ResourceId) string {
	if r.SpaceId == "" {
		return fmt.Sprintf("%s!%s", r.StorageId, r.OpaqueId)
	} else if r.OpaqueId == "" {
		return fmt.Sprintf("%s$%s", r.StorageId, r.SpaceId)
	}
	return fmt.Sprintf("%s$%s!%s", r.StorageId, r.SpaceId, r.OpaqueId)
}

// EncodeResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
// If space_id is not set, it will be calculated from the path.
// If no path or space_id is set, an error will be returned
func EncodeResourceInfo(md *provider.ResourceInfo) (spaceId string, err error) {
	if md.Id.SpaceId != "" {
		return fmt.Sprintf("%s$%s!%s", md.Id.StorageId, md.Id.SpaceId, md.Id.OpaqueId), nil
	} else if md.Path != "" {
		encodedPath := PathToSpaceID(md.Path)
		return fmt.Sprintf("%s$%s!%s", md.Id.StorageId, encodedPath, md.Id.OpaqueId), nil
	} else {
		return "", errors.New("resourceInfo must contain a spaceID or a path")
	}
}

// EncodeStorageSpaceID encodes storage ID and path to create an identifier
// in the format <storage_id>$base32(<path>), where base32(<path>) is the space ID.
func EncodeStorageSpaceID(storageID, path string) string {
	if path == "" {
		return storageID
	}

	encodedPath := PathToSpaceID(path)
	return fmt.Sprintf("%s$%s", storageID, encodedPath)
}

// If the path given is a subfolder of a space,
// then the ID of that space will be returned.
// If it is not a subfolder, then the space-encoding (base32)
// of this full path will be returned.
func PathToSpaceID(path string) string {
	paths := strings.Split(path, string(os.PathSeparator))
	if len(paths) < spacesLevel(path) {
		return EncodeSpaceID(path)
	}
	spacesPath := strings.Join(paths[:spacesLevel(path)], string(os.PathSeparator))
	return EncodeSpaceID(spacesPath)
}

// TODO: for now, we hardcoded this. But this will not be necessary anymore
// once all storage providers decorate all the returned ResourceInfos with a space ID,
// because we won't need to do path -> space_id anymore

// Returns how many parts of the path belong to the space identifier
// - For EOS user/project, this is 5 ((1)/(2)eos/(3)user/(4)u/(5)user)
func spacesLevel(path string) int {
	if strings.HasPrefix(path, "/eos/user") || strings.HasPrefix(path, "/eos/project") {
		return 5
	} else if strings.HasPrefix(path, "/winspaces") {
		// e.g. /winspaces/c/copstest-doyle
		return 4
	} else {
		// a safe default for all other eos paths (e.g. /eos/experiment etc)
		return 3
	}
}

func RelativePathToSpaceID(info *provider.ResourceInfo) string {
	return strings.TrimPrefix(info.Path, info.Id.SpaceId)
}

func ResourceIdToString(id *provider.ResourceId) string {
	return fmt.Sprintf("%s+%s", id.StorageId, id.OpaqueId)
}

func ResourceIdFromString(s string) (*provider.ResourceId, error) {
	parts := strings.Split(s, "+")
	if len(parts) != 2 {
		return nil, fmt.Errorf("string does not have right format: should be storageid!opaqueid, got %s", s)
	}
	return &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}, nil
}
