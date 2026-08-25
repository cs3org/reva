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

package ocdav

import (
	"context"
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

// handleFilesRoot serves the bare dav-files root
// (/remote.php/dav/files/{user}/). It lists the CS3 namespace root "/" through
// the gateway (the top-level roots such as "eos" and "winspaces"), and adds a
// "home" alias for the user's home, which is sharded per user and so is not a
// listable entry at "/". Descending into any entry resolves to the real CS3
// path in dav.go. Only PROPFIND is meaningful here.
func (s *svc) handleFilesRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx).With().Str("handler", "files-root").Logger()

	switch r.Method {
	case http.MethodOptions:
		s.handleOptions(w, r)
		return
	case http.MethodHead:
		s.handleHead(ctx, w, r, &provider.Reference{Path: "/"}, log)
		return
	case MethodPropfind:
		// handled below
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		log.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// Reuse the normal path propfind flow on the namespace root: it does the
	// gateway Stat + ListContainer and handles depth and errors.
	ref := &provider.Reference{Path: "/"}
	parentInfo, resourceInfos, hrefBase, ok := s.getResourceInfos(ctx, w, r, pf, ref, log)
	if !ok {
		return
	}

	// Add the "home" alias, unless the root listing already exposes one.
	depth := r.Header.Get(HeaderDepth)
	if depth == "" {
		depth = "1"
	}
	if depth != "0" {
		if home := s.homeChild(ctx); home != nil && !hasChildNamed(resourceInfos, "home") {
			resourceInfos = append(resourceInfos, home)
		}
	}

	s.propfindResponse(ctx, w, r, "/", hrefBase, pf, parentInfo, resourceInfos, log)
}

// homeChild resolves the user's home through the gateway and relabels it to
// appear as "home" under the dav-files root, keeping the real id, etag and
// permissions so the client can browse and sync it.
func (s *svc) homeChild(ctx context.Context) *provider.ResourceInfo {
	client, err := s.getClient()
	if err != nil {
		return nil
	}
	home, err := client.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil || home.Status.Code != rpc.Code_CODE_OK {
		return nil
	}
	res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: home.Path}})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		return nil
	}
	res.Info.Path = "/home"
	return res.Info
}

func hasChildNamed(infos []*provider.ResourceInfo, name string) bool {
	for _, ri := range infos {
		if path.Base(ri.Path) == name {
			return true
		}
	}
	return false
}
