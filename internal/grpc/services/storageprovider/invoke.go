// Copyright 2018-2026 CERN
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

package storageprovider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/invoke"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/storage"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
)

// The storage provider is an invoke.Invokable: through the Admin API an
// operator reads and sets the quota of the spaces this provider serves. A
// provider only knows its own mount, so the caller addresses the node serving
// the path; the mount is advertised in the registry metadata (mount_id,
// mount_path) and by the mount invocation.

// RegistryMetadata advertises the mount this provider serves, so a client can
// pick the instance serving a path.
func (s *service) RegistryMetadata() map[string]string {
	m := map[string]string{svcregistry.MetaMountPath: s.mountPath}
	if s.mountID != "" {
		m[svcregistry.MetaMountID] = s.mountID
	}
	return m
}

func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("mount", "Report the mount this provider serves.").
		Handle(s.invokeMount)
	s.Set.Add("quota_get", "Read the quota of a space served by this provider: the project quota of the path, or with style=uid the quota of a user.").
		Arg("path", "the space, under this provider's mount").
		OptArg("style", "gid99 (default): the project quota set on the path; uid: the quota of user on the path or on the storage's quota node").
		OptArg("user", "the user whose quota to read, for style=uid").
		Handle(s.invokeQuotaGet)
	s.Set.Add("quota_set", "Set the quota of a space served by this provider, in bytes and inodes as EOS stores them. Address the node serving the path.").
		Mutating().
		Arg("path", "the space, under this provider's mount").
		Arg("max_bytes", "the volume quota, in bytes").
		Arg("max_inodes", "the inode quota").
		OptArg("style", "gid99 (default): the project quota set on the path; uid: the quota of user").
		OptArg("user", "the user whose quota to set, for style=uid").
		Handle(s.invokeQuotaSet)
}

func (s *service) invokeMount(_ context.Context, _ invoke.Args) (invoke.Result, error) {
	_, supported := s.storage.(storage.QuotaAdmin)
	return invoke.Result{
		"mount_path":  s.mountPath,
		"mount_id":    s.mountID,
		"driver":      s.conf.Driver,
		"space_type":  s.conf.ProvidesSpaceType,
		"quota_admin": supported,
	}, nil
}

func (s *service) invokeQuotaGet(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	qa, ref, err := s.quotaRef(ctx, a)
	if err != nil {
		return nil, err
	}
	qi, err := qa.GetQuotaFor(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.quotaResult(a, ref, qi), nil
}

func (s *service) invokeQuotaSet(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	qa, ref, err := s.quotaRef(ctx, a)
	if err != nil {
		return nil, err
	}
	maxBytes, err := strconv.ParseUint(a.String("max_bytes"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("max_bytes must be a number of bytes, not %q", a.String("max_bytes"))
	}
	maxInodes, err := strconv.ParseUint(a.String("max_inodes"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("max_inodes must be a number, not %q", a.String("max_inodes"))
	}
	err = qa.SetQuotaFor(ctx, ref, maxBytes, maxInodes)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "storageprovider.quota_set",
		Actor:   admin.Actor(ctx),
		Target:  a.String("path"),
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  map[string]string{"mount_id": s.mountID, "style": style(a), "user": a.String("user"), "max_bytes": a.String("max_bytes"), "max_inodes": a.String("max_inodes")},
	})
	if err != nil {
		return nil, err
	}
	qi, err := qa.GetQuotaFor(ctx, ref)
	if err != nil {
		return invoke.Result{"path": a.String("path"), "set": true, "warning": "the quota is set but could not be read back: " + err.Error()}, nil
	}
	r := s.quotaResult(a, ref, qi)
	r["set"] = true
	return r, nil
}

// quotaRef validates the path against this provider's mount and builds the
// storage-relative quota reference for the requested style.
func (s *service) quotaRef(ctx context.Context, a invoke.Args) (storage.QuotaAdmin, storage.QuotaRef, error) {
	qa, ok := s.storage.(storage.QuotaAdmin)
	if !ok {
		return nil, storage.QuotaRef{}, fmt.Errorf("storageprovider: the %s driver does not support quota administration", s.conf.Driver)
	}
	rel, err := s.relative(a.String("path"))
	if err != nil {
		return nil, storage.QuotaRef{}, err
	}
	ref := storage.QuotaRef{Path: rel}
	switch style(a) {
	case "gid99", "project":
		ref.GID = eosclient.ProjectQuotaGID
	case "uid", "user":
		u, err := s.lookupUser(ctx, a.String("user"))
		if err != nil {
			return nil, storage.QuotaRef{}, fmt.Errorf("style=uid needs user=<username>: %w", err)
		}
		if u.GetUidNumber() == 0 {
			return nil, storage.QuotaRef{}, fmt.Errorf("user %s has no uid", a.String("user"))
		}
		ref.UID = strconv.FormatInt(u.GetUidNumber(), 10)
		ref.GID = strconv.FormatInt(u.GetGidNumber(), 10)
	default:
		return nil, storage.QuotaRef{}, fmt.Errorf("style must be gid99 or uid, not %q", a.String("style"))
	}
	return qa, ref, nil
}

// relative strips this provider's mount off an absolute path, refusing a path
// served elsewhere so that a wrongly addressed node fails loudly.
func (s *service) relative(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("path must be absolute, not %q", p)
	}
	mount := strings.TrimSuffix(s.mountPath, "/")
	switch {
	case mount == "" || mount == "/":
		return p, nil
	case p == mount:
		return "/", nil
	case strings.HasPrefix(p, mount+"/"):
		return strings.TrimPrefix(p, mount), nil
	}
	return "", fmt.Errorf("%s is not under the mount %s served by this instance (%s)", p, s.mountPath, s.mountID)
}

func (s *service) quotaResult(a invoke.Args, ref storage.QuotaRef, qi *storage.QuotaInfo) invoke.Result {
	return invoke.Result{
		"path":         a.String("path"),
		"mount_id":     s.mountID,
		"style":        style(a),
		"uid":          ref.UID,
		"gid":          ref.GID,
		"total_bytes":  qi.TotalBytes,
		"used_bytes":   qi.UsedBytes,
		"total_inodes": qi.TotalInodes,
		"used_inodes":  qi.UsedInodes,
	}
}

// lookupUser resolves a username; a field so tests can substitute it.
func (s *service) lookupUser(ctx context.Context, username string) (*userpb.User, error) {
	if s.lookup != nil {
		return s.lookup(ctx, username)
	}
	return admin.Lookup(ctx, username)
}

func style(a invoke.Args) string {
	if !a.Has("style") || a.String("style") == "" {
		return "gid99"
	}
	return strings.ToLower(a.String("style"))
}
