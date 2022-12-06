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

package eoswrapper

import (
	"bytes"
	"context"
	"io"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/eosfs"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("eoswrapper", New)
}

const (
	eosProjectsNamespace = "/eos/project"

	// We can use a regex for these, but that might have inferior performance.
	projectSpaceGroupsPrefix      = "cernbox-project-"
	projectSpaceAdminGroupsSuffix = "-admins"
)

type wrapper struct {
	storage.FS
	conf            *eosfs.Config
	mountIDTemplate *template.Template
}

func parseConfig(m map[string]interface{}) (*eosfs.Config, string, error) {
	c := &eosfs.Config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, "", err
	}

	// default to version invariance if not configured
	if _, ok := m["version_invariant"]; !ok {
		c.VersionInvariant = true
	}

	// allow recycle operations for project spaces
	if !c.EnableHome && strings.HasPrefix(c.Namespace, eosProjectsNamespace) {
		c.AllowPathRecycleOperations = true
		c.ImpersonateOwnerforRevisions = true
	}

	t, ok := m["mount_id_template"].(string)
	if !ok || t == "" {
		t = "eoshome-{{ trimAll \"/\" .Path | substr 0 1 }}"
	}

	return c, t, nil
}

// New returns an implementation of the storage.FS interface that forms a wrapper
// around separate connections to EOS.
func New(m map[string]interface{}) (storage.FS, error) {
	c, t, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	eos, err := eosfs.NewEOSFS(c)
	if err != nil {
		return nil, err
	}

	mountIDTemplate, err := template.New("mountID").Funcs(sprig.TxtFuncMap()).Parse(t)
	if err != nil {
		return nil, err
	}

	return &wrapper{FS: eos, conf: c, mountIDTemplate: mountIDTemplate}, nil
}

// We need to override the two methods, GetMD and ListFolder to fill the
// StorageId in the ResourceInfo objects.

func (w *wrapper) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	res, err := w.FS.GetMD(ctx, ref, mdKeys)
	if err != nil {
		return nil, err
	}

	// We need to extract the mount ID based on the mapping template.
	//
	// Take the first letter of the resource path after the namespace has been removed.
	// If it's empty, leave it empty to be filled by storageprovider.
	res.Id.StorageId = w.getMountID(ctx, res)

	if err = w.setProjectSharingPermissions(ctx, res); err != nil {
		return nil, err
	}

	return res, nil
}

func (w *wrapper) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	res, err := w.FS.ListFolder(ctx, ref, mdKeys)
	if err != nil {
		return nil, err
	}
	for _, r := range res {
		r.Id.StorageId = w.getMountID(ctx, r)
		if err = w.setProjectSharingPermissions(ctx, r); err != nil {
			continue
		}
	}
	return res, nil
}

func (w *wrapper) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	if err := w.userIsProjectAdmin(ctx, ref); err != nil {
		return nil, err
	}

	return w.FS.ListRevisions(ctx, ref)
}

func (w *wrapper) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	if err := w.userIsProjectAdmin(ctx, ref); err != nil {
		return nil, err
	}

	return w.FS.DownloadRevision(ctx, ref, revisionKey)
}

func (w *wrapper) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	if err := w.userIsProjectAdmin(ctx, ref); err != nil {
		return err
	}

	return w.FS.RestoreRevision(ctx, ref, revisionKey)
}

func (w *wrapper) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	// This is only allowed for project space admins
	if strings.HasPrefix(w.conf.Namespace, eosProjectsNamespace) {
		if err := w.userIsProjectAdmin(ctx, ref); err != nil {
			return err
		}
		return w.FS.DenyGrant(ctx, ref, g)
	}

	return errtypes.NotSupported("eos: deny grant is only enabled for project spaces")
}

func (w *wrapper) getMountID(ctx context.Context, r *provider.ResourceInfo) string {
	if r == nil {
		return ""
	}
	b := bytes.Buffer{}
	if err := w.mountIDTemplate.Execute(&b, r); err != nil {
		return ""
	}
	return b.String()
}

func (w *wrapper) setProjectSharingPermissions(ctx context.Context, r *provider.ResourceInfo) error {
	// Check if this storage provider corresponds to a project spaces instance
	if strings.HasPrefix(w.conf.Namespace, eosProjectsNamespace) {
		// Extract project name from the path resembling /c/cernbox or /c/cernbox/minutes/..
		parts := strings.SplitN(r.Path, "/", 4)
		if len(parts) != 4 && len(parts) != 3 {
			// The request might be for / or /$letter
			// Nothing to do in that case
			return nil
		}
		adminGroup := projectSpaceGroupsPrefix + parts[2] + projectSpaceAdminGroupsSuffix
		user := ctxpkg.ContextMustGetUser(ctx)

		for _, g := range user.Groups {
			if g == adminGroup {
				r.PermissionSet.AddGrant = true
				r.PermissionSet.RemoveGrant = true
				r.PermissionSet.UpdateGrant = true
				r.PermissionSet.ListGrants = true
				r.PermissionSet.GetQuota = true
				r.PermissionSet.DenyGrant = true
				return nil
			}
		}
	}
	return nil
}

func (w *wrapper) userIsProjectAdmin(ctx context.Context, ref *provider.Reference) error {
	// Check if this storage provider corresponds to a project spaces instance
	if !strings.HasPrefix(w.conf.Namespace, eosProjectsNamespace) {
		return nil
	}

	res, err := w.FS.GetMD(ctx, ref, nil)
	if err != nil {
		return err
	}

	// Extract project name from the path resembling /c/cernbox or /c/cernbox/minutes/..
	parts := strings.SplitN(res.Path, "/", 4)
	if len(parts) != 4 && len(parts) != 3 {
		// The request might be for / or /$letter
		// Nothing to do in that case
		return nil
	}
	adminGroup := projectSpaceGroupsPrefix + parts[2] + projectSpaceAdminGroupsSuffix
	user := ctxpkg.ContextMustGetUser(ctx)

	for _, g := range user.Groups {
		if g == adminGroup {
			return nil
		}
	}

	return errtypes.PermissionDenied("eosfs: project spaces revisions can only be accessed by admins")
}
