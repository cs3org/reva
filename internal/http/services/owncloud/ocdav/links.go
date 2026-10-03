// Copyright 2018-2024 CERN
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
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/downloader"
)

// Links (symbolic links and Windows .lnk shortcuts) are exposed in PROPFIND
// through two properties, returned only when explicitly requested:
//
//   - oc:link-type: "symlink" or "lnk", for any resource recognized as a link;
//   - oc:link-target: the href of the target, only if the link is navigable.
//
// A link is navigable when:
//
//   - its target is a relative path that, once resolved against the folder
//     containing the link, stays within the root exposed to the client: the
//     space root, or the shared folder for public links; or
//   - for symbolic links only, its target is an absolute path that the gateway
//     resolves, on behalf of the user, to a resource in any storage space. The
//     href then points to that space (/spaces/<storage$space>/...).
//
// Any other link (e.g. pointing to a path local to the storage, to a resource
// the user can not access, or escaping the root through ..) is returned as a
// plain, opaque file, and its target is not disclosed. Absolute targets are
// never resolved within public links.
//
// The target is never followed server side: clients are expected to navigate
// to the target href, where the usual permission checks apply.

const (
	_propLinkType   = "link-type"
	_propLinkTarget = "link-target"

	linkTypeSymlink = "symlink"
	linkTypeLnk     = "lnk"

	// maximum number of .lnk files read or link targets stat'ed in parallel
	// for a single PROPFIND
	linkResolveConcurrency = 8
)

type linkInfo struct {
	Type string
	// TargetPath is the CS3 path of a target within the same root as the link,
	// whose href is relative to the one of the link.
	TargetPath string
	// TargetHref is the href of a target in another space.
	// If both TargetPath and TargetHref are empty the link is opaque.
	TargetHref string
}

func isLinkProp(n *propfindProps) bool {
	for _, p := range *n {
		if p.Space == _nsOwncloud && (p.Local == _propLinkType || p.Local == _propLinkTarget) {
			return true
		}
	}
	return false
}

func isLnkFile(md *provider.ResourceInfo) bool {
	return md.Type == provider.ResourceType_RESOURCE_TYPE_FILE && strings.EqualFold(path.Ext(md.Path), ".lnk")
}

func isPublicLinkRequest(ctx context.Context) bool {
	token, ok := ctx.Value(ctxPublicLink).(string)
	return ok && token != ""
}

// linkRoot returns the CS3 path above which the relative targets of the links
// contained in md cannot go.
func linkRoot(ctx context.Context, md *provider.ResourceInfo) (string, bool) {
	if token, ok := ctx.Value(ctxPublicLink).(string); ok && token != "" {
		// paths in public links look like /public/<token>/...
		parts := strings.SplitAfter(md.Path, "/"+token)
		if len(parts) < 2 || (parts[1] != "" && !strings.HasPrefix(parts[1], "/")) {
			return "", false
		}
		return parts[0], true
	}
	if md.Id == nil {
		return "", false
	}
	root, err := spaces.DecodeSpaceID(md.Id.SpaceId)
	if err != nil || !path.IsAbs(root) {
		return "", false
	}
	return path.Clean(root), true
}

func isWithin(p, root string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}

// resolveLinkTarget returns the CS3 path of target, as stored in the link at
// linkPath, if target is relative and does not escape root.
func resolveLinkTarget(linkPath, target, root string, windows bool) (string, bool) {
	if windows {
		target = strings.ReplaceAll(target, `\`, "/")
		// drive letters (C:...) are absolute, or at least not relative to the link
		if len(target) >= 2 && target[1] == ':' {
			return "", false
		}
	}
	if target == "" || path.IsAbs(target) || strings.ContainsRune(target, 0) {
		return "", false
	}
	resolved := path.Join(path.Dir(linkPath), target)
	if resolved == path.Clean(linkPath) || !isWithin(resolved, root) {
		return "", false
	}
	return resolved, true
}

// spaceHref returns the href of info in the spaces endpoint rooted at davBase,
// e.g. /remote.php/dav/spaces/<storage$space>/path/relative/to/space.
func spaceHref(davBase string, info *provider.ResourceInfo) (string, bool) {
	if davBase == "" || info.GetId().GetStorageId() == "" || info.GetId().GetSpaceId() == "" {
		return "", false
	}
	spacePath, err := spaces.DecodeSpaceID(info.Id.SpaceId)
	if err != nil || !path.IsAbs(spacePath) || !isWithin(path.Clean(info.Path), path.Clean(spacePath)) {
		return "", false
	}
	relativePath, err := filepath.Rel(encodePath(spacePath), encodePath(info.Path))
	if err != nil {
		return "", false
	}
	href, err := url.JoinPath(encodePath(path.Join(davBase, "spaces")),
		spaces.EncodeStorageSpaceID(info.Id.StorageId, info.Id.SpaceId), relativePath)
	if err != nil {
		return "", false
	}
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		href, _ = url.JoinPath(href, "/")
	}
	return href, true
}

// resolveLinks returns the link information for the links among mds, keyed
// by path. It reads the content of .lnk files and stats absolute symlink
// targets, in parallel, if enabled.
func (s *svc) resolveLinks(ctx context.Context, mds []*provider.ResourceInfo) map[string]*linkInfo {
	if !s.c.EnableLinkTargets {
		return nil
	}
	links := map[string]*linkInfo{}
	// slow resolutions, which need to contact the storage
	var tasks []func() (string, *linkInfo)
	for _, md := range mds {
		switch {
		case md.Type == provider.ResourceType_RESOURCE_TYPE_SYMLINK:
			li := &linkInfo{Type: linkTypeSymlink}
			links[md.Path] = li
			root, ok := linkRoot(ctx, md)
			if !path.IsAbs(md.Target) {
				if ok {
					li.TargetPath, _ = resolveLinkTarget(md.Path, md.Target, root, false)
				}
				continue
			}
			if isPublicLinkRequest(ctx) {
				continue
			}
			target := path.Clean(md.Target)
			if target == path.Clean(md.Path) {
				continue
			}
			if ok && isWithin(target, root) {
				li.TargetPath = target
				continue
			}
			md := md
			tasks = append(tasks, func() (string, *linkInfo) {
				href, _ := s.statLinkTarget(ctx, target)
				return md.Path, &linkInfo{Type: linkTypeSymlink, TargetHref: href}
			})
		case isLnkFile(md) && md.Size <= uint64(s.c.MaxShortcutSize) &&
			(md.PermissionSet == nil || md.PermissionSet.InitiateFileDownload):
			md := md
			tasks = append(tasks, func() (string, *linkInfo) {
				sl := s.readShellLink(ctx, md)
				if sl == nil {
					// not a valid shortcut: just a file with a .lnk extension
					return "", nil
				}
				li := &linkInfo{Type: linkTypeLnk}
				// the relative path is what makes a shortcut portable; an absolute
				// target (local or UNC path) alone is not resolvable for our clients
				if root, ok := linkRoot(ctx, md); ok && sl.RelativePath != "" {
					li.TargetPath, _ = resolveLinkTarget(md.Path, sl.RelativePath, root, true)
				}
				return md.Path, li
			})
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, linkResolveConcurrency)
	for _, task := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(task func() (string, *linkInfo)) {
			defer func() { <-sem; wg.Done() }()
			if p, li := task(); li != nil {
				mu.Lock()
				links[p] = li
				mu.Unlock()
			}
		}(task)
	}
	wg.Wait()
	return links
}

// statLinkTarget stats the absolute target of a symlink on behalf of the
// current user, and returns its href in the spaces endpoint if it exists.
func (s *svc) statLinkTarget(ctx context.Context, target string) (string, bool) {
	log := appctx.GetLogger(ctx)
	davBase, _ := ctx.Value(ctxKeyDavBaseURI).(string)
	if davBase == "" {
		// e.g. the legacy /webdav endpoint, which does not expose spaces
		return "", false
	}
	client, err := service.Gateway(ctx)
	if err != nil {
		log.Error().Err(err).Msg("ocdav: error getting gateway client to stat link target")
		return "", false
	}
	res, err := client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: target}})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		// no storage for this path, not found or not accessible: the link stays opaque
		log.Debug().Err(err).Str("target", target).Interface("status", res.GetStatus()).Msg("ocdav: link target not resolved")
		return "", false
	}
	return spaceHref(davBase, res.Info)
}

// readShellLink returns the parsed content of a .lnk file, or nil if it can
// not be read or parsed. Results are cached by resource id and etag.
func (s *svc) readShellLink(ctx context.Context, md *provider.ResourceInfo) *shellLink {
	log := appctx.GetLogger(ctx)
	key := md.Path + ":" + md.Etag
	if md.Id != nil {
		key = spaces.EncodeToStringifiedResourceID(md.Id) + ":" + md.Etag
	}
	if v, err := s.lnkCache.Get(key); err == nil {
		sl, _ := v.(*shellLink)
		return sl
	}

	client, err := service.Gateway(ctx)
	if err != nil {
		log.Error().Err(err).Msg("ocdav: error getting gateway client to read shortcut")
		return nil
	}
	r, err := downloader.NewDownloader(client, s.client).Download(ctx, md.Path, "")
	if err != nil {
		log.Debug().Err(err).Str("path", md.Path).Msg("ocdav: could not read shortcut")
		return nil
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, s.c.MaxShortcutSize))
	if err != nil {
		log.Debug().Err(err).Str("path", md.Path).Msg("ocdav: could not read shortcut")
		return nil
	}

	sl, err := parseShellLink(data)
	if err != nil {
		sl = nil
	}
	// negative results are cached as well, as the etag changes with the content
	_ = s.lnkCache.Set(key, sl)
	return sl
}
