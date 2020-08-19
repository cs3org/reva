// Copyright 2018-2020 CERN
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

package gateway

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/token"
	"github.com/pkg/errors"
	"github.com/studio-b12/gowebdav"
)

type webdavEndpoint struct {
	filePath string
	endpoint string
	token    string
}

func (s *svc) webdavRefStat(ctx context.Context, targetURL string, nameQueries ...string) (*provider.ResourceInfo, error) {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return nil, err
	}

	ep, err := extractEndpointInfo(targetURL)
	if err != nil {
		return nil, err
	}
	c := gowebdav.NewClient(ep.endpoint, "", "")
	c.SetHeader(token.TokenHeader, ep.token)

	// TODO(ishank011): We need to call PROPFIND ourselves as we need to retrieve
	// ownloud-specific fields to get the resource ID and permissions.
	info, err := c.Stat(ep.filePath)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("gateway: error statting %s at the webdav endpoint: %s", ep.filePath, ep.endpoint))
	}
	return normalize(info.(*gowebdav.File)), nil
}

func (s *svc) webdavRefLs(ctx context.Context, targetURL string, nameQueries ...string) ([]*provider.ResourceInfo, error) {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return nil, err
	}

	ep, err := extractEndpointInfo(targetURL)
	if err != nil {
		return nil, err
	}
	c := gowebdav.NewClient(ep.endpoint, "", "")
	c.SetHeader(token.TokenHeader, ep.token)

	// TODO(ishank011): We need to call PROPFIND ourselves as we need to retrieve
	// ownloud-specific fields to get the resource ID and permissions.
	infos, err := c.ReadDir(ep.filePath)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("gateway: error listing %s at the webdav endpoint: %s", ep.filePath, ep.endpoint))
	}

	mds := []*provider.ResourceInfo{}
	for _, fi := range infos {
		info := fi.(gowebdav.File)
		mds = append(mds, normalize(&info))
	}
	return mds, nil
}

func (s *svc) webdavRefMkdir(ctx context.Context, targetURL string, nameQueries ...string) error {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return err
	}

	ep, err := extractEndpointInfo(targetURL)
	if err != nil {
		return err
	}
	c := gowebdav.NewClient(ep.endpoint, "", "")
	c.SetHeader(token.TokenHeader, ep.token)

	err = c.Mkdir(ep.filePath, 0700)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("gateway: error creating dir %s at the webdav endpoint: %s", ep.filePath, ep.endpoint))
	}
	return nil
}

func (s *svc) webdavRefMove(ctx context.Context, targetURL, src, destination string) error {
	srcURL, err := appendNameQuery(targetURL, src)
	if err != nil {
		return err
	}
	srcEP, err := extractEndpointInfo(srcURL)
	if err != nil {
		return err
	}

	destURL, err := appendNameQuery(targetURL, destination)
	if err != nil {
		return err
	}
	destEP, err := extractEndpointInfo(destURL)
	if err != nil {
		return err
	}

	c := gowebdav.NewClient(srcEP.endpoint, "", "")
	c.SetHeader(token.TokenHeader, srcEP.token)

	err = c.Rename(srcEP.filePath, destEP.filePath, true)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("gateway: error renaming %s to %s at the webdav endpoint: %s", srcEP.filePath, destEP.filePath, srcEP.endpoint))
	}
	return nil
}

func (s *svc) webdavRefDelete(ctx context.Context, targetURL string, nameQueries ...string) error {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return err
	}

	ep, err := extractEndpointInfo(targetURL)
	if err != nil {
		return err
	}
	c := gowebdav.NewClient(ep.endpoint, "", "")
	c.SetHeader(token.TokenHeader, ep.token)

	err = c.Remove(ep.filePath)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("gateway: error removing %s at the webdav endpoint: %s", ep.filePath, ep.endpoint))
	}
	return nil
}

func (s *svc) webdavRefTransferEndpoint(ctx context.Context, targetURL string, nameQueries ...string) (string, *types.Opaque, error) {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return "", nil, err
	}

	ep, err := extractEndpointInfo(targetURL)
	if err != nil {
		return "", nil, err
	}

	return ep.endpoint, &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"webdav-file-path": {
				Decoder: "plain",
				Value:   []byte(ep.filePath),
			},
			"webdav-token": {
				Decoder: "plain",
				Value:   []byte(ep.token),
			},
		},
	}, nil
}

func normalize(info *gowebdav.File) *provider.ResourceInfo {
	return &provider.ResourceInfo{
		// TODO(ishank011): Add Id, PermissionSet, Owner
		Path:     info.Path(),
		Type:     getResourceType(info.IsDir()),
		Etag:     info.ETag(),
		MimeType: info.ContentType(),
		Size:     uint64(info.Size()),
		Mtime: &types.Timestamp{
			Seconds: uint64(info.ModTime().Unix()),
		},
	}
}

func extractEndpointInfo(targetURL string) (*webdavEndpoint, error) {
	if targetURL == "" {
		return nil, errors.New("gateway: ref target is an empty uri")
	}

	uri, err := url.Parse(targetURL)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error parsing target uri: "+targetURL)
	}
	if uri.Scheme != "webdav" {
		return nil, errtypes.NotSupported("ref target does not have the webdav scheme")
	}

	parts := strings.SplitN(uri.Opaque, "@", 2)
	if len(parts) < 2 {
		err := errors.New("gateway: webdav ref does not follow the layout token@webdav_endpoint?name " + targetURL)
		return nil, err
	}
	m, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error parsing target resource name")
	}

	return &webdavEndpoint{
		filePath: m["name"][0],
		endpoint: parts[1],
		token:    parts[0],
	}, nil
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func appendNameQuery(targetURL string, nameQueries ...string) (string, error) {
	uri, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	q, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return "", err
	}
	name := append([]string{q["name"][0]}, nameQueries...)
	q.Set("name", path.Join(name...))
	uri.RawQuery = q.Encode()
	return uri.String(), nil
}
