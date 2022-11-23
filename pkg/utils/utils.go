// Copyright 2018-2021 CERN
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

package utils

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os/user"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/registry"
	"github.com/cs3org/reva/pkg/registry/memory"

	// gocritic is disabled because google.golang.org/protobuf/proto does not provide a method to convert MessageV1 to MessageV2.
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
	matchEmail    = regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	// GlobalRegistry configures a service registry globally accessible. It defaults to a memory registry. The usage of
	// globals is not encouraged, and this is a workaround until the PR is out of a draft state.
	GlobalRegistry registry.Registry = memory.New(map[string]interface{}{})
)

// Skip  evaluates whether a source endpoint contains any of the prefixes.
// i.e: /a/b/c/d/e contains prefix /a/b/c.
func Skip(source string, prefixes []string) bool {
	for i := range prefixes {
		if strings.HasPrefix(source, prefixes[i]) {
			return true
		}
	}
	return false
}

// GetClientIP retrieves the client IP from incoming requests.
func GetClientIP(r *http.Request) (string, error) {
	var clientIP string
	forwarded := r.Header.Get("X-FORWARDED-FOR")

	if forwarded != "" {
		clientIP = forwarded
	} else {
		if ip, _, err := net.SplitHostPort(r.RemoteAddr); err != nil {
			ipObj := net.ParseIP(r.RemoteAddr)
			if ipObj == nil {
				return "", err
			}
			clientIP = ipObj.String()
		} else {
			clientIP = ip
		}
	}
	return clientIP, nil
}

// ToSnakeCase converts a CamelCase string to a snake_case string.
func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

// ResolvePath converts relative local paths to absolute paths.
func ResolvePath(path string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	homeDir := usr.HomeDir

	if path == "~" {
		path = homeDir
	} else if strings.HasPrefix(path, "~/") {
		path = filepath.Join(homeDir, path[2:])
	}

	return filepath.Abs(path)
}

// RandString is a helper to create tokens.
func RandString(n int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	var l = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = l[rand.Intn(len(l))]
	}
	return string(b)
}

// TSToUnixNano converts a protobuf Timestamp to uint64
// with nanoseconds resolution.
func TSToUnixNano(ts *types.Timestamp) uint64 {
	return uint64(time.Unix(int64(ts.Seconds), int64(ts.Nanos)).UnixNano())
}

// TSToTime converts a protobuf Timestamp to Go's time.Time.
func TSToTime(ts *types.Timestamp) time.Time {
	return time.Unix(int64(ts.Seconds), int64(ts.Nanos))
}

// LaterTS returns the timestamp which occurs later.
func LaterTS(t1 *types.Timestamp, t2 *types.Timestamp) *types.Timestamp {
	if TSToUnixNano(t1) > TSToUnixNano(t2) {
		return t1
	}
	return t2
}

// ExtractGranteeID returns the ID, user or group, set in the GranteeId object.
func ExtractGranteeID(grantee *provider.Grantee) (*userpb.UserId, *grouppb.GroupId) {
	switch t := grantee.Id.(type) {
	case *provider.Grantee_UserId:
		return t.UserId, nil
	case *provider.Grantee_GroupId:
		return nil, t.GroupId
	default:
		return nil, nil
	}
}

// UserEqual returns whether two users have the same field values.
func UserEqual(u, v *userpb.UserId) bool {
	return u != nil && v != nil && u.Idp == v.Idp && u.OpaqueId == v.OpaqueId
}

// GroupEqual returns whether two groups have the same field values.
func GroupEqual(u, v *grouppb.GroupId) bool {
	return u != nil && v != nil && u.Idp == v.Idp && u.OpaqueId == v.OpaqueId
}

// ResourceIDEqual returns whether two resources have the same field values.
func ResourceIDEqual(u, v *provider.ResourceId) bool {
	return u != nil && v != nil && u.StorageId == v.StorageId && u.OpaqueId == v.OpaqueId
}

// ResourceEqual returns whether two resources have the same field values.
func ResourceEqual(u, v *provider.Reference) bool {
	return u != nil && v != nil && u.Path == v.Path && ((u.ResourceId == nil && v.ResourceId == nil) || (ResourceIDEqual(u.ResourceId, v.ResourceId)))
}

// GranteeEqual returns whether two grantees have the same field values.
func GranteeEqual(u, v *provider.Grantee) bool {
	if u == nil || v == nil {
		return false
	}
	uu, ug := ExtractGranteeID(u)
	vu, vg := ExtractGranteeID(v)
	return u.Type == v.Type && (UserEqual(uu, vu) || GroupEqual(ug, vg))
}

// IsEmailValid checks whether the provided email has a valid format.
func IsEmailValid(e string) bool {
	if len(e) < 3 || len(e) > 254 {
		return false
	}
	return matchEmail.MatchString(e)
}

// IsValidWebAddress checks whether the provided address is a valid URL.
func IsValidWebAddress(address string) bool {
	_, err := url.ParseRequestURI(address)
	return err == nil
}

// IsValidPhoneNumber checks whether the provided phone number has a valid format.
func IsValidPhoneNumber(number string) bool {
	re := regexp.MustCompile(`^(?:(?:\(?(?:00|\+)([1-4]\d\d|[1-9]\d?)\)?)?[\-\.\ \\\/]?)?((?:\(?\d{1,}\)?[\-\.\ \\\/]?){0,})(?:[\-\.\ \\\/]?(?:#|ext\.?|extension|x)[\-\.\ \\\/]?(\d+))?$`)
	return re.MatchString(number)
}

// IsValidName cheks if the given name doesn't contain any non-alpha, space or dash characters.
func IsValidName(name string) bool {
	re := regexp.MustCompile(`^[A-Za-z\s\-]*$`)
	return re.MatchString(name)
}

// MarshalProtoV1ToJSON marshals a proto V1 message to a JSON byte array
// TODO: update this once we start using V2 in CS3APIs.
func MarshalProtoV1ToJSON(m proto.Message) ([]byte, error) {
	mV2 := proto.MessageV2(m)
	return protojson.Marshal(mV2)
}

// UnmarshalJSONToProtoV1 decodes a JSON byte array to a specified proto message type
// TODO: update this once we start using V2 in CS3APIs.
func UnmarshalJSONToProtoV1(b []byte, m proto.Message) error {
	mV2 := proto.MessageV2(m)
	if err := protojson.Unmarshal(b, mV2); err != nil {
		return err
	}
	return nil
}

// IsRelativeReference returns true if the given reference qualifies as relative
// when the resource id is set and the path starts with a .
//
// TODO(corby): Currently if the path begins with a dot, the ResourceId is set but has empty storageId and OpaqueId
// then the reference is still being viewed as relative. We need to check if we want that because in some
// places we might not want to set both StorageId and OpaqueId so we can't do a hard check if they are set.
func IsRelativeReference(ref *provider.Reference) bool {
	return ref.ResourceId != nil && strings.HasPrefix(ref.Path, ".")
}

// IsAbsoluteReference returns true if the given reference qualifies as absolute
// when either only the resource id is set or only the path is set and starts with /
//
// TODO(corby): Currently if the path is empty, the ResourceId is set but has empty storageId and OpaqueId
// then the reference is still being viewed as absolute. We need to check if we want that because in some
// places we might not want to set both StorageId and OpaqueId so we can't do a hard check if they are set.
func IsAbsoluteReference(ref *provider.Reference) bool {
	return (ref.ResourceId != nil && ref.Path == "") || (ref.ResourceId == nil) && strings.HasPrefix(ref.Path, "/")
}

// IsAbsolutePathReference returns true if the given reference qualifies as a global path
// when only the path is set and starts with /.
func IsAbsolutePathReference(ref *provider.Reference) bool {
	return ref.ResourceId == nil && strings.HasPrefix(ref.Path, "/")
}

// MakeRelativePath prefixes the path with a . to use it in a relative reference.
func MakeRelativePath(p string) string {
	p = path.Join("/", p)

	if p == "/" {
		return "."
	}
	return "." + p
}

// UserTypeMap translates account type string to CS3 UserType.
func UserTypeMap(accountType string) userpb.UserType {
	var t userpb.UserType
	switch accountType {
	case "primary":
		t = userpb.UserType_USER_TYPE_PRIMARY
	case "secondary":
		t = userpb.UserType_USER_TYPE_SECONDARY
	case "service":
		t = userpb.UserType_USER_TYPE_SERVICE
	case "application":
		t = userpb.UserType_USER_TYPE_APPLICATION
	case "guest":
		t = userpb.UserType_USER_TYPE_GUEST
	case "federated":
		t = userpb.UserType_USER_TYPE_FEDERATED
	case "lightweight":
		t = userpb.UserType_USER_TYPE_LIGHTWEIGHT
	}
	return t
}

// UserTypeToString translates CS3 UserType to user-readable string.
func UserTypeToString(accountType userpb.UserType) string {
	var t string
	switch accountType {
	case userpb.UserType_USER_TYPE_PRIMARY:
		t = "primary"
	case userpb.UserType_USER_TYPE_SECONDARY:
		t = "secondary"
	case userpb.UserType_USER_TYPE_SERVICE:
		t = "service"
	case userpb.UserType_USER_TYPE_APPLICATION:
		t = "application"
	case userpb.UserType_USER_TYPE_GUEST:
		t = "guest"
	case userpb.UserType_USER_TYPE_FEDERATED:
		t = "federated"
	case userpb.UserType_USER_TYPE_LIGHTWEIGHT:
		t = "lightweight"
	}
	return t
}

// SplitStorageSpaceID can be used to split `storagespaceid` into `storageid` and `nodeid`
// Currently they are built using `<storageid>!<nodeid>` in the decomposedfs, but other drivers might return different ids.
// any place in the code that relies on this function should instead use the storage registry to look up the responsible storage provider.
// Note: This would in effect change the storage registry into a storage space registry.
func SplitStorageSpaceID(ssid string) (storageid, nodeid string, err error) {
	// query that specific storage provider
	parts := strings.SplitN(ssid, "!", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("storage space id must be separated by '!'")
	}
	return parts[0], parts[1], nil
}

// ParseStorageSpaceReference parses a string into a spaces reference.
// The expected format is `<storageid>!<nodeid>/<path>`.
func ParseStorageSpaceReference(sRef string) (provider.Reference, error) {
	parts := strings.SplitN(sRef, "/", 2)

	storageid, nodeid, err := SplitStorageSpaceID(parts[0])
	if err != nil {
		return provider.Reference{}, err
	}

	var relPath string
	if len(parts) == 2 {
		relPath = parts[1]
	}

	return provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: storageid,
			OpaqueId:  nodeid,
		},
		Path: MakeRelativePath(relPath),
	}, nil
}

// GetViewMode converts a human-readable string to a view mode for opening a resource in an app.
func GetViewMode(viewMode string) gateway.OpenInAppRequest_ViewMode {
	switch viewMode {
	case "view":
		return gateway.OpenInAppRequest_VIEW_MODE_VIEW_ONLY
	case "read":
		return gateway.OpenInAppRequest_VIEW_MODE_READ_ONLY
	case "write":
		return gateway.OpenInAppRequest_VIEW_MODE_READ_WRITE
	case "preview":
		return gateway.OpenInAppRequest_VIEW_MODE_PREVIEW
	default:
		return gateway.OpenInAppRequest_VIEW_MODE_INVALID
	}
}

// HasPublicShareRole return true if the user has a public share role.
// If yes, the string is the type of role, viewer, editor or uploader.
func HasPublicShareRole(u *userpb.User) (string, bool) {
	if u.Opaque == nil {
		return "", false
	}
	if publicShare, ok := u.Opaque.Map["public-share-role"]; ok {
		return string(publicShare.Value), true
	}
	return "", false
}

// HasPermissions returns true if all permissions defined in the stuict toCheck
// are set in the target.
func HasPermissions(target, toCheck *provider.ResourcePermissions) bool {
	targetStruct := reflect.ValueOf(target).Elem()
	toCheckStruct := reflect.ValueOf(toCheck).Elem()

	for i := 0; i < toCheckStruct.NumField(); i++ {
		fieldToCheck := toCheckStruct.Field(i)
		if fieldToCheck.Kind() == reflect.Bool && fieldToCheck.Bool() && !targetStruct.Field(i).Bool() {
			return false
		}
	}
	return true
}

// UserIsLightweight returns true if the user is a lightweith
// or federated account.
func UserIsLightweight(u *userpb.User) bool {
	return u.Id.Type == userpb.UserType_USER_TYPE_FEDERATED ||
		u.Id.Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT
}
