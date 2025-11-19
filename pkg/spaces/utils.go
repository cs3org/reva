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
	"path/filepath"
	"runtime"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// EncodeStorageSpaceID encodes storage ID and space ID.
// In case of empty space ID, the path is used to create an identifier
// in the format <storage_id>$base32(<path>), where base32(<path>) is the space ID.
func EncodeStorageSpaceID(storageID, path string) string {
	if path == "" {
		return storageID
	}

	encodedPath := PathToSpaceID(path)
	return ConcatStorageSpaceID(storageID, encodedPath)
}

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

func ConcatStorageSpaceID(storageID, spaceID string) string {
	return fmt.Sprintf("%s$%s", storageID, spaceID)
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

func EncodeOCMShareID(ShareID string) string {
	return fmt.Sprintf("ocm-received$%s", base32.StdEncoding.EncodeToString([]byte("/ocm-received/"+ShareID)))
}

// EncodeResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
// If space_id or opaque_id is not set on the ResourceId,
// then this part will not be encoded
func EncodeResourceID(r *provider.ResourceId) string {
	var encoded string
	if r.SpaceId == "" {
		encoded = fmt.Sprintf("%s!%s", r.StorageId, r.OpaqueId)
		fmt.Fprintf(os.Stderr, "[DEBUG] EncodeResourceID: storage=%q, space=(empty), opaque=%q -> %q\n", 
			r.StorageId, r.OpaqueId, encoded)
		fmt.Fprintf(os.Stderr, "[DEBUG] EncodeResourceID STACK (space empty):\n")
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		fmt.Fprintf(os.Stderr, "%s\n", buf[:n])
	} else if r.OpaqueId == "" {
		encoded = fmt.Sprintf("%s$%s", r.StorageId, r.SpaceId)
		fmt.Fprintf(os.Stderr, "[DEBUG] EncodeResourceID: storage=%q, space=%q, opaque=(empty) -> %q\n", 
			r.StorageId, r.SpaceId, encoded)
	} else {
		encoded = fmt.Sprintf("%s$%s!%s", r.StorageId, r.SpaceId, r.OpaqueId)
		fmt.Fprintf(os.Stderr, "[DEBUG] EncodeResourceID: storage=%q, space=%q, opaque=%q -> %q\n", 
			r.StorageId, r.SpaceId, r.OpaqueId, encoded)
	}
	return encoded
}

// Decode resourceID returns the components of the space ID.
// The resource ID is expected to be in the form of <storage_id>$base32(<path>)!<item_id>.
func DecodeResourceID(raw string) (storageID, spacePath, itemID string, ok bool) {
	// The input is expected to be in the form of <storage_id>$base32(<path>)!<item_id>
	s := strings.SplitN(raw, "!", 2)
	if len(s) != 2 {
		return "", "", "", false
	}
	itemID = s[1]
	storageID, spacePath, ok = DecodeStorageSpaceID(s[0])
	return storageID, spacePath, itemID, ok
}

// ParseResourceID converts the encoded resource id in a CS3API ResourceId.
func ParseResourceID(raw string) (*provider.ResourceId, bool) {
	storageID, path, itemID, ok := DecodeResourceID(raw)
	if !ok {
		return nil, false
	}

	spaceID := PathToSpaceID(path)

	return &provider.ResourceId{
		StorageId: storageID,
		SpaceId:   spaceID,
		OpaqueId:  itemID,
	}, true
}

// EncodeResourceInfo encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
// If space_id is not set, it will be calculated from the path.
// If no path or space_id is set, an error will be returned
func EncodeResourceInfo(info *provider.ResourceInfo) (spaceId string, err error) {
	if info.Id.SpaceId != "" {
		return fmt.Sprintf("%s$%s!%s", info.Id.StorageId, info.Id.SpaceId, info.Id.OpaqueId), nil
	} else if info.Path != "" {
		encodedPath := PathToSpaceID(info.Path)
		return fmt.Sprintf("%s$%s!%s", info.Id.StorageId, encodedPath, info.Id.OpaqueId), nil
	} else {
		return "", errors.New("resourceInfo must contain a spaceID or a path")
	}
}

// If the path given is a subfolder of a space,
// then the ID of that space will be returned.
// If it is not a subfolder, then the space-encoding (base32)
// of this full path will be returned.
func PathToSpaceID(path string) string {
	paths := strings.Split(path, string(os.PathSeparator))
	level := spacesLevel(path)
	var spacesPath string
	var spaceID string
	
	if len(paths) < level {
		spacesPath = path
		spaceID = EncodeSpaceID(path)
	} else {
		spacesPath = strings.Join(paths[:level], string(os.PathSeparator))
		spaceID = EncodeSpaceID(spacesPath)
	}
	
	// Debug logging
	fmt.Fprintf(os.Stderr, "[DEBUG] PathToSpaceID: input=%q, level=%d, paths=%d, spacesPath=%q, spaceID=%q\n", 
		path, level, len(paths), spacesPath, spaceID)
	
	return spaceID
}

// TODO: for now, we hardcoded this. But this will not be necessary anymore
// once all storage providers decorate all the returned ResourceInfos with a space ID,
// because we won't need to do path -> space_id anymore

// Returns how many parts of the path belong to the space identifier
// - For EOS user/project, this is 5 ((1)/(2)eos/(3)user/(4)u/(5)user)
func spacesLevel(path string) int {
	if strings.HasPrefix(path, "/eos/user") || strings.HasPrefix(path, "/eos/project") {
		return 5
	} else if strings.HasPrefix(path, "/winspaces") || strings.HasPrefix(path, "/eos/media") || strings.HasPrefix(path, "/eos/atlas") {
		// e.g. /winspaces/c/copstest-doyle
		return 4
	} else {
		// a safe default for all other eos paths (e.g. /eos/experiment etc)
		return 3
	}
}

// Returns the path relative to the space root.
func PathRelativeToSpaceRoot(info *provider.ResourceInfo) (relativePath string, err error) {
	spacePath, err := ResourceToSpacePath(info)
	if err != nil {
		return "", err
	}

	return filepath.Rel(spacePath, info.Path)
}

func ResourceToSpacePath(info *provider.ResourceInfo) (path string, err error) {
	if info.Id.SpaceId == "" {
		return "", errors.New("resourceInfo must contain a space ID")
	}
	_, spacePath, ok := DecodeStorageSpaceID(fmt.Sprintf("%s$%s", info.Id.StorageId, info.Id.SpaceId))
	if !ok {
		return "", fmt.Errorf("failed to decode storage space ID: %s", fmt.Sprintf("%s$%s", info.Id.StorageId, info.Id.SpaceId))
	}

	return spacePath, nil
}

func ResourceIdToString(id *provider.ResourceId) string {
	return fmt.Sprintf("%s!%s", id.StorageId, id.OpaqueId)
}

func ResourceIdFromString(s string) (*provider.ResourceId, error) {
	parts := strings.Split(s, "!")
	if len(parts) != 2 {
		return nil, fmt.Errorf("string does not have right format: should be storageid!opaqueid, got %s", s)
	}
	return &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}, nil
}
