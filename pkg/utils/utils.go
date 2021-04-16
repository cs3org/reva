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
	"math/rand"
	"net"
	"net/http"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/registry"
	"github.com/cs3org/reva/pkg/registry/memory"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
	matchEmail    = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	// GlobalRegistry configures a service registry globally accessible. It defaults to a memory registry. The usage of
	// globals is not encouraged, and this is a workaround until the PR is out of a draft state.
	GlobalRegistry registry.Registry = memory.New(map[string]interface{}{})
)

// Skip  evaluates whether a source endpoint contains any of the prefixes.
// i.e: /a/b/c/d/e contains prefix /a/b/c
func Skip(source string, prefixes []string) bool {
	for i := range prefixes {
		if strings.HasPrefix(source, prefixes[i]) {
			return true
		}
	}
	return false
}

// GetClientIP retrieves the client IP from incoming requests
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

// ResolvePath converts relative local paths to absolute paths
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

	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return path, nil
}

// RandString is a helper to create tokens.
func RandString(n int) string {
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

// ExtractGranteeID returns the ID, user or group, set in the GranteeId object
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

// ResourceEqual returns whether two resources have the same field values.
func ResourceEqual(u, v *provider.ResourceId) bool {
	return u != nil && v != nil && u.StorageId == v.StorageId && u.OpaqueId == v.OpaqueId
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

// MarshalProtoV1ToJSON marshals a proto V1 message to a JSON byte array
// TODO: update this once we start using V2 in CS3APIs
func MarshalProtoV1ToJSON(m proto.Message) ([]byte, error) {
	mV2 := proto.MessageV2(m)
	return protojson.Marshal(mV2)
}

// UnmarshalJSONToProtoV1 decodes a JSON byte array to a specified proto message type
// TODO: update this once we start using V2 in CS3APIs
func UnmarshalJSONToProtoV1(b []byte, m proto.Message) error {
	mV2 := proto.MessageV2(m)
	if err := protojson.Unmarshal(b, mV2); err != nil {
		return err
	}
	return nil
}
