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
	"net/url"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
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

func extractEndpointInfo(ri *provider.ResourceInfo) (*webdavEndpoint, error) {
	if ri.Type != provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		panic("gateway: calling handleWebdavRefStat on a non reference type:" + ri.String())
	}
	// reference types MUST have a target resource id.
	if ri.Target == "" {
		err := errors.New("gateway: ref target is an empty uri")
		return nil, err
	}

	uri, err := url.Parse(ri.Target)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error parsing target uri: "+ri.Target)
	}
	if uri.Scheme != "webdav" {
		return nil, errtypes.NotSupported("ref target does not have the webdav scheme")
	}

	parts := strings.SplitN(uri.Opaque, "@", 2)
	if len(parts) < 2 {
		err := errors.New("gateway: webdav ref does not follow the layout token@webdav_endpoint?name " + ri.Target)
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

func (s *svc) handleWebdavRefStat(ctx context.Context, ri *provider.ResourceInfo) (*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	ep, err := extractEndpointInfo(ri)
	log.Info().Msgf("ep: %+v", ep)
	if err != nil {
		return nil, err
	}
	c := gowebdav.NewClient(ep.endpoint, "", "")
	c.SetHeader(token.TokenHeader, ep.token)

	// We need to call PROPFIND ourselves as we need ownloud-specific fields
	// to read the resource ID and permissions.
	info, err := c.Stat(ep.filePath)
	fileInfo := info.(*gowebdav.File)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling stat at the webdav endpoint: "+ep.endpoint)
	}

	md := &provider.ResourceInfo{
		// Add Id, PermissionSet, Owner
		Path:     fileInfo.Path(),
		Type:     getResourceType(fileInfo.IsDir()),
		Etag:     fileInfo.ETag(),
		MimeType: fileInfo.ContentType(),
		Size:     uint64(fileInfo.Size()),
		Mtime: &types.Timestamp{
			Seconds: uint64(fileInfo.ModTime().Unix()),
		},
	}
	log.Info().Msgf("md: %+v", md)

	return md, nil
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}
