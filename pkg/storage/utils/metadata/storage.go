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

package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	revactx "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/utils"
	"google.golang.org/grpc/metadata"
)

type Storage interface {
	Init(ctx context.Context) (err error)
	SimpleUpload(ctx context.Context, uploadpath string, content []byte) error
	SimpleDownload(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error

	ListContainer(ctx context.Context, path string) ([]*provider.ResourceInfo, error)

	CreateSymlink(ctx context.Context, oldname, newname string) error
	ResolveSymlink(ctx context.Context, name string) (string, error)

	MakeDirIfNotExist(ctx context.Context, name string) error
}

type CS3 struct {
	storageProvider   provider.ProviderAPIClient
	serviceUser       *user.User
	dataGatewayClient *http.Client
	SpaceRoot         *provider.ResourceId
}

// New returns a new storage instance
func NewCS3(providerAddr string, serviceUser *user.User) (s Storage, err error) {
	p, err := pool.GetStorageProviderServiceClient(providerAddr)
	if err != nil {
		return nil, err
	}

	c := http.DefaultClient

	return &CS3{
		storageProvider:   p,
		dataGatewayClient: c,
		serviceUser:       serviceUser,
	}, nil
}

// Init creates the metadata space
func (cs3 *CS3) Init(ctx context.Context) (err error) {
	// FIXME change CS3 api to allow sending a space id
	cssr, err := cs3.storageProvider.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
		Opaque: &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				"spaceid": {
					Decoder: "plain",
					Value:   []byte(cs3.serviceUser.Id.OpaqueId),
				},
			},
		},
		Owner: cs3.serviceUser,
		Name:  "Metadata",
		Type:  "metadata",
	})
	switch {
	case err != nil:
		return err
	case cssr.Status.Code == rpc.Code_CODE_OK:
		cs3.SpaceRoot = cssr.StorageSpace.Root
	case cssr.Status.Code == rpc.Code_CODE_ALREADY_EXISTS:
		// TODO make CreateStorageSpace return existing space?
		cs3.SpaceRoot = &provider.ResourceId{StorageId: cs3.serviceUser.Id.OpaqueId, OpaqueId: cs3.serviceUser.Id.OpaqueId}
	default:
		return errtypes.NewErrtypeFromStatus(cssr.Status)
	}
	return nil
}

func (cs3 *CS3) SimpleUpload(ctx context.Context, uploadpath string, content []byte) error {
	ref := provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{
			ResourceId: cs3.SpaceRoot,
			Path:       utils.MakeRelativePath(uploadpath),
		},
	}

	res, err := cs3.storageProvider.InitiateFileUpload(ctx, &ref)
	if err != nil {
		return err
	}

	var endpoint string

	for _, proto := range res.GetProtocols() {
		if proto.Protocol == "simple" {
			endpoint = proto.GetUploadEndpoint()
			break
		}
	}
	if endpoint == "" {
		return errors.New("metadata storage doesn't support the simple upload protocol")
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(content))
	if err != nil {
		return err
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	req.Header.Add(revactx.TokenHeader, md.Get(revactx.TokenHeader)[0])
	resp, err := cs3.dataGatewayClient.Do(req)
	if err != nil {
		return err
	}
	if err = resp.Body.Close(); err != nil {
		return err
	}
	return nil
}

func (cs3 *CS3) SimpleDownload(ctx context.Context, downloadpath string) (content []byte, err error) {
	dreq := provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			ResourceId: cs3.SpaceRoot,
			Path:       utils.MakeRelativePath(downloadpath),
		},
	}

	res, err := cs3.storageProvider.InitiateFileDownload(ctx, &dreq)
	if err != nil {
		return []byte{}, errtypes.NotFound(dreq.Ref.Path)
	}

	var endpoint string

	for _, proto := range res.GetProtocols() {
		if proto.Protocol == "spaces" {
			endpoint = proto.GetDownloadEndpoint()
			break
		}
	}
	if endpoint == "" {
		return []byte{}, errors.New("metadata storage doesn't support the spaces download protocol")
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return []byte{}, err
	}

	md, _ := metadata.FromOutgoingContext(ctx)
	req.Header.Add(revactx.TokenHeader, md.Get(revactx.TokenHeader)[0])
	resp, err := cs3.dataGatewayClient.Do(req)
	if err != nil {
		return []byte{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return []byte{}, errtypes.NotFound(dreq.Ref.Path)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	if err = resp.Body.Close(); err != nil {
		return []byte{}, err
	}

	return b, nil
}

// Delete deletes a path
func (cs3 *CS3) Delete(ctx context.Context, path string) error {
	res, err := cs3.storageProvider.Delete(ctx, &provider.DeleteRequest{
		Ref: &provider.Reference{
			ResourceId: cs3.SpaceRoot,
			Path:       utils.MakeRelativePath(path),
		},
	})
	if err != nil {
		return err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("error deleting path: %v", path)
	}

	return nil
}

// ListContainer returns the resource infos in a given directory
func (cs3 *CS3) ListContainer(ctx context.Context, path string) ([]*provider.ResourceInfo, error) {
	res, err := cs3.storageProvider.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			ResourceId: cs3.SpaceRoot,
			Path:       utils.MakeRelativePath(path),
		},
	})

	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("error listing directory: %v", path)
	}

	return res.Infos, nil
}

// MakeDirIfNotExist will create a root node in the metadata storage. Requires an authenticated context.
func (cs3 *CS3) MakeDirIfNotExist(ctx context.Context, folder string) error {
	var rootPathRef = &provider.Reference{
		ResourceId: cs3.SpaceRoot,
		Path:       utils.MakeRelativePath(folder),
	}

	resp, err := cs3.storageProvider.Stat(ctx, &provider.StatRequest{
		Ref: rootPathRef,
	})

	if err != nil {
		return err
	}

	if resp.Status.Code == rpc.Code_CODE_NOT_FOUND {
		_, err := cs3.storageProvider.CreateContainer(ctx, &provider.CreateContainerRequest{
			Ref: rootPathRef,
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func (cs3 *CS3) CreateSymlink(ctx context.Context, oldname, newname string) error {
	if _, err := cs3.ResolveSymlink(ctx, newname); err == nil {
		return os.ErrExist
	}

	err := cs3.SimpleUpload(ctx, newname, []byte(oldname))
	if err != nil {
		return err
	}
	return nil
}

func (cs3 *CS3) ResolveSymlink(ctx context.Context, name string) (string, error) {
	b, err := cs3.SimpleDownload(ctx, name)
	if err != nil {
		if errors.Is(err, errtypes.NotFound("")) {
			return "", os.ErrNotExist
		}
		return "", err
	}

	return string(b), err
}
