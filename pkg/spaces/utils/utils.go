package utils

import (
	"encoding/base32"
	"fmt"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// DecodeSpaceID returns the components of the space ID.
// The space ID is expected to be in the format <storage_id>$<base32(<path>).
func DecodeSpaceID(raw string) (storageID, path string, ok bool) {
	// The input is expected to be in the form of <storage_id>$<base32(<path>)
	s := strings.SplitN(raw, "$", 2)
	if len(s) != 2 {
		return
	}

	storageID = s[0]
	encodedPath := s[1]
	p, err := base32.StdEncoding.DecodeString(encodedPath)
	if err != nil {
		return
	}

	path = string(p)
	ok = true
	return
}

// Decode resourceID returns the components of the space ID.
// The resource ID is expected to be in the form of <storage_id>$<base32(<path>)!<item_id>.
func DecodeResourceID(raw string) (storageID, path, itemID string, ok bool) {
	// The input is expected to be in the form of <storage_id>$<base32(<path>)!<item_id>
	s := strings.SplitN(raw, "!", 2)
	if len(s) != 2 {
		return
	}
	itemID = s[1]
	storageID, path, ok = DecodeSpaceID(s[0])
	return
}

// EncodeResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
func EncodeResourceID(r *provider.ResourceId) string {
	return fmt.Sprintf("%s$%s!%s", r.StorageId, r.SpaceId, r.OpaqueId)
}

// EncodeSpaceID encodes storage ID and path to create a space ID,
// in the format <storage_id>$<base32(<path>).
func EncodeSpaceID(storageID, path string) string {
	encodedPath := base32.StdEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("%s$%s", storageID, encodedPath)
}
