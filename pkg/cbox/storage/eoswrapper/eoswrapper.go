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

package eoshome

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/eosfs"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("eoswrapper", New)
}

type wrapper struct {
	storage.FS
	mountIDTemplate string
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

	t, ok := m["mount_id_template"].(string)
	if !ok || t == "" {
		t = "eoshome-{{substr 0 1 .Id.OpaqueId}}"
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

	return &wrapper{FS: eos, mountIDTemplate: t}, nil
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
	// Take the owner of the resource as the user to decide the mapping.
	// If that is not present, leave it empty to be filled by storageprovider.
	if res != nil && res.Owner != nil && res.Owner.OpaqueId != "" {
		res.Id.StorageId = w.getMountID(ctx, res.Owner)
	}
	return res, nil

}

func (w *wrapper) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	res, err := w.FS.ListFolder(ctx, ref, mdKeys)
	if err != nil {
		return nil, err
	}
	for _, r := range res {
		if r != nil && r.Owner != nil && r.Owner.OpaqueId != "" {
			r.Id.StorageId = w.getMountID(ctx, r.Owner)
		}
	}
	return res, nil
}

func (w *wrapper) getMountID(ctx context.Context, u *userpb.UserId) string {
	return templates.WithUser(&userpb.User{Id: u}, w.mountIDTemplate)
}
