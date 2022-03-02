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

package gateway

import (
	"context"
	"path"
	"strings"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/storage/utils/etag"
)

func (s *svc) getSharedFolder(ctx context.Context) string {
	return path.Join("/", s.c.ShareFolder)
}

// check if the path contains the prefix of the shared folder
func (s *svc) inSharedFolder(ctx context.Context, p string) bool {
	sharedFolder := s.getSharedFolder(ctx)
	return strings.HasPrefix(p, sharedFolder)
}

// /MyShares/
func (s *svc) isSharedFolder(ctx context.Context, p string) bool {
	return p == s.getSharedFolder(ctx)
}

// /MyShares/photos/
func (s *svc) isShareName(ctx context.Context, p string) bool {
	sharedFolder := s.getSharedFolder(ctx)
	rel := strings.Trim(strings.TrimPrefix(p, sharedFolder), "/")
	return strings.HasPrefix(p, sharedFolder) && len(strings.Split(rel, "/")) == 1
}

// /MyShares/photos/Ibiza/beach.png
func (s *svc) isShareChild(ctx context.Context, p string) bool {
	sharedFolder := s.getSharedFolder(ctx)
	rel := strings.Trim(strings.TrimPrefix(p, sharedFolder), "/")
	return strings.HasPrefix(p, sharedFolder) && len(strings.Split(rel, "/")) > 1
}

// path must contain a share path with share children, if not it will panic.
// should be called after checking isShareChild == true
func (s *svc) splitShare(ctx context.Context, p string) (string, string) {
	sharedFolder := s.getSharedFolder(ctx)
	p = strings.Trim(strings.TrimPrefix(p, sharedFolder), "/")
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		panic("gateway: path for splitShare does not contain 2 elements:" + p)
	}

	shareName := path.Join(sharedFolder, parts[0])
	shareChild := path.Join("/", parts[1])
	return shareName, shareChild
}

func (s *svc) statSharesFolder(ctx context.Context) (*provider.StatResponse, error) {
	statRes, err := s.stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: s.getSharedFolder(ctx),
		},
	})
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating shares folder"),
		}, nil
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: statRes.Status,
		}, nil
	}

	lsRes, err := s.listSharesFolder(ctx)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error listing shares folder"),
		}, nil
	}
	if lsRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: lsRes.Status,
		}, nil
	}

	etagCacheKey := statRes.Info.Owner.OpaqueId + ":" + statRes.Info.Path
	if resEtag, err := s.etagCache.Get(etagCacheKey); err == nil {
		statRes.Info.Etag = resEtag.(string)
	} else {
		statRes.Info.Etag = etag.GenerateEtagFromResources(statRes.Info, lsRes.Infos)
		if s.c.EtagCacheTTL > 0 {
			_ = s.etagCache.Set(etagCacheKey, statRes.Info.Etag)
		}
	}
	return statRes, nil
}

func (s *svc) listSharesFolder(ctx context.Context) (*provider.ListContainerResponse, error) {
	lcr, err := s.listContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			Path: s.getSharedFolder(ctx),
		},
	})
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "gateway: error listing shared folder"),
		}, nil
	}
	if lcr.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: lcr.Status,
		}, nil
	}
	checkedInfos := make([]*provider.ResourceInfo, 0)
	for i := range lcr.Infos {
		info, protocol, err := s.checkRef(ctx, lcr.Infos[i])
		if err != nil {
			// Create status to log the proper messages
			// This might arise when the shared resource has been moved to the recycle bin
			// or when the resource was unshared, but the share reference was not removed
			status.NewStatusFromErrType(ctx, "error resolving reference "+lcr.Infos[i].Target, err)
			continue
		}

		if protocol == "webdav" {
			info, err = s.webdavRefStat(ctx, lcr.Infos[i].Target)
			if err != nil {
				// This might arise when the webdav token has expired
				continue
			}
		}

		// It should be possible to delete and move share references, so expose all possible permissions
		info.PermissionSet = conversions.NewManagerRole().CS3ResourcePermissions()
		info.Path = lcr.Infos[i].GetPath()
		checkedInfos = append(checkedInfos, info)
	}
	lcr.Infos = checkedInfos

	return lcr, nil
}
