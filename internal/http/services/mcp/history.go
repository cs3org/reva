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

package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	labels "github.com/cs3org/go-cs3apis/cs3/labels/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
)

func (s *svc) registerHistoryTools() {
	addTool(s, toolSpec{
		name:        toolListVersions,
		description: "List the previous versions of a file, which restore_version can bring back. path is absolute.",
		permissions: []string{scope.PermVersionsList},
		readOnly:    true,
	}, s.listVersions)

	addTool(s, toolSpec{
		name:        toolRestoreVersion,
		description: "Restore a previous version of a file, by the key list_versions returned. The current content becomes a version in turn. path is absolute.",
		permissions: []string{scope.PermVersionsRestore},
	}, s.restoreVersion)

	addTool(s, toolSpec{
		name: toolListTrash,
		description: "List the recently deleted items in the recycle bin of a space, newest first. space is the absolute path of the space, or of any folder in it; " +
			"by default the user's home. When next_cursor is set, call again with it as cursor.",
		permissions: []string{scope.PermTrashList},
		readOnly:    true,
	}, s.listTrash)

	addTool(s, toolSpec{
		name: toolRestoreFromTrash,
		description: "Restore a deleted item, by the key list_trash returned. It goes back to where it was deleted from; " +
			"destination asks for another absolute path, where the storage supports it. The result says where the item is.",
		permissions: []string{scope.PermTrashRestore},
	}, s.restoreFromTrash)

	addTool(s, toolSpec{
		name:        toolGetLabels,
		description: "Get the labels (tags) the user put on a file or folder. path is absolute.",
		permissions: []string{scope.PermLabelsRead},
		readOnly:    true,
	}, s.getLabels)

	addTool(s, toolSpec{
		name:        toolSetLabels,
		description: "Set the labels (tags) of a file or folder to exactly the given list: missing ones are added, others removed. An empty list removes all. path is absolute.",
		// A label is stored as metadata of the resource as well.
		permissions: []string{scope.PermLabelsWrite, scope.PermFilesWrite},
	}, s.setLabels)
}

type version struct {
	Key   string `json:"key" jsonschema:"pass it to restore_version"`
	Size  uint64 `json:"size"`
	Mtime string `json:"mtime"`
	Etag  string `json:"etag,omitempty"`
}

type listVersionsOut struct {
	Path     string    `json:"path"`
	Versions []version `json:"versions"`
}

func (s *svc) listVersions(c *call, in pathIn) (listVersionsOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return listVersionsOut{}, err
	}
	c.path = p
	res, err := c.gw.ListFileVersions(c.ctx, &provider.ListFileVersionsRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return listVersionsOut{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return listVersionsOut{}, err
	}
	out := listVersionsOut{Path: p, Versions: []version{}}
	for _, v := range res.GetVersions() {
		out.Versions = append(out.Versions, version{
			Key:   v.GetKey(),
			Size:  v.GetSize(),
			Mtime: formatTime(&types.Timestamp{Seconds: v.GetMtime()}),
			Etag:  v.GetEtag(),
		})
	}
	slices.SortFunc(out.Versions, func(a, b version) int { return strings.Compare(b.Mtime, a.Mtime) })
	return out, nil
}

type restoreVersionIn struct {
	Path       string `json:"path" jsonschema:"absolute path of the file"`
	VersionKey string `json:"version_key" jsonschema:"key of the version, from list_versions"`
}

func (s *svc) restoreVersion(c *call, in restoreVersionIn) (entry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return entry{}, err
	}
	c.path = p
	if in.VersionKey == "" {
		return entry{}, newError(codeInvalidArgument, "version_key is required")
	}
	res, err := c.gw.RestoreFileVersion(c.ctx, &provider.RestoreFileVersionRequest{Ref: &provider.Reference{Path: p}, Key: in.VersionKey})
	if err != nil {
		return entry{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return entry{}, err
	}
	info, err := c.stat(p)
	if err != nil {
		return entry{}, err
	}
	return s.entry(info), nil
}

type trashItem struct {
	Key     string `json:"key" jsonschema:"pass it to restore_from_trash"`
	Path    string `json:"path" jsonschema:"where it was deleted from"`
	Type    string `json:"type"`
	Size    uint64 `json:"size"`
	Deleted string `json:"deleted" jsonschema:"when it was deleted, RFC 3339"`
}

type listTrashIn struct {
	Space  string `json:"space,omitempty" jsonschema:"absolute path of the space, or of a folder in it; the user's home by default"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page"`
}

type listTrashOut struct {
	Items      []trashItem `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty" jsonschema:"set when there are more items: pass it as cursor"`
}

// trashKey is what a client gets as the key of a deleted item: the reference
// the recycle bin was listed with, and the key in it.
type trashKey struct {
	Ref string `json:"r"`
	Key string `json:"k"`
}

func (k trashKey) encode() string {
	b, _ := json.Marshal(k)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeTrashKey(s string) (trashKey, error) {
	var k trashKey
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &k)
	}
	if err != nil || k.Ref == "" || k.Key == "" {
		return k, newError(codeInvalidArgument, "key is not one list_trash returned")
	}
	return k, nil
}

// trashRef is the path to list the recycle bin of a space with. The recycle
// bin is the one of the space holding the path, so an app password restricted
// to folders inside the space lists it through one of them.
func (c *call) trashRef(space string) (string, error) {
	if space == "" {
		res, err := c.gw.GetHome(c.ctx, &provider.GetHomeRequest{})
		if err != nil {
			return "", asToolError(err, "home")
		}
		if err := checkStatus(res.GetStatus(), "home"); err != nil {
			return "", err
		}
		space = res.GetPath()
	}
	if path.IsAbs(space) {
		space = path.Clean(space)
		for _, r := range c.ap.Roots {
			if !c.ap.UnderRoots(space) && strings.HasPrefix(r, strings.TrimSuffix(space, "/")+"/") {
				return r, nil
			}
		}
	}
	return c.absPath(space, "space")
}

func (c *call) listRecycle(ref string) ([]*provider.RecycleItem, error) {
	res, err := c.gw.ListRecycle(c.ctx, &provider.ListRecycleRequest{Ref: &provider.Reference{Path: ref}})
	if err != nil {
		return nil, asToolError(err, "recycle bin of "+ref)
	}
	if err := checkStatus(res.GetStatus(), "recycle bin of "+ref); err != nil {
		return nil, err
	}
	return res.GetRecycleItems(), nil
}

func (s *svc) listTrash(c *call, in listTrashIn) (listTrashOut, error) {
	ref, err := c.trashRef(in.Space)
	if err != nil {
		return listTrashOut{}, err
	}
	c.path = ref
	items, err := c.listRecycle(ref)
	if err != nil {
		return listTrashOut{}, err
	}
	items = slices.DeleteFunc(items, func(it *provider.RecycleItem) bool { return !c.ap.UnderRoots(it.GetRef().GetPath()) })
	// Newest first.
	newestFirst := func(it *provider.RecycleItem) string {
		return fmt.Sprintf("%016x/%s", ^it.GetDeletionTime().GetSeconds(), it.GetKey())
	}
	items, next, err := page(items, newestFirst, in.Cursor, s.c.ListMaxEntries)
	if err != nil {
		return listTrashOut{}, err
	}
	out := listTrashOut{Items: make([]trashItem, 0, len(items)), NextCursor: next}
	for _, it := range items {
		out.Items = append(out.Items, trashItem{
			Key:     trashKey{Ref: ref, Key: it.GetKey()}.encode(),
			Path:    it.GetRef().GetPath(),
			Type:    resourceType(it.GetType()),
			Size:    it.GetSize(),
			Deleted: formatTime(it.GetDeletionTime()),
		})
	}
	return out, nil
}

type restoreFromTrashIn struct {
	Key         string `json:"key" jsonschema:"key of the item, from list_trash"`
	Destination string `json:"destination,omitempty" jsonschema:"absolute path to restore it to, where the storage supports it; where it was deleted from by default"`
}

type restoreFromTrashOut struct {
	Path string `json:"path" jsonschema:"where the item is now"`
}

func (s *svc) restoreFromTrash(c *call, in restoreFromTrashIn) (restoreFromTrashOut, error) {
	k, err := decodeTrashKey(in.Key)
	if err != nil {
		return restoreFromTrashOut{}, err
	}
	ref, err := c.absPath(k.Ref, "key")
	if err != nil {
		return restoreFromTrashOut{}, err
	}
	// Where the item was deleted from is looked up again rather than taken
	// from the key: a storage may restore there whatever the destination.
	items, err := c.listRecycle(ref)
	if err != nil {
		return restoreFromTrashOut{}, err
	}
	i := slices.IndexFunc(items, func(it *provider.RecycleItem) bool { return it.GetKey() == k.Key })
	if i < 0 {
		return restoreFromTrashOut{}, newError(codeNotFound, "the item is no longer in the recycle bin")
	}
	original, err := c.absPath(items[i].GetRef().GetPath(), "the deleted item")
	if err != nil {
		return restoreFromTrashOut{}, err
	}
	dst := original
	if in.Destination != "" {
		if dst, err = c.absPath(in.Destination, "destination"); err != nil {
			return restoreFromTrashOut{}, err
		}
	}
	c.path, c.destination = original, dst
	res, err := c.gw.RestoreRecycleItem(c.ctx, &provider.RestoreRecycleItemRequest{
		Ref:        &provider.Reference{Path: ref},
		Key:        k.Key,
		RestoreRef: &provider.Reference{Path: dst},
	})
	if err != nil {
		return restoreFromTrashOut{}, asToolError(err, original)
	}
	if err := checkStatus(res.GetStatus(), original); err != nil {
		return restoreFromTrashOut{}, err
	}
	if dst != original {
		if _, err := c.stat(dst); err != nil {
			return restoreFromTrashOut{Path: original}, nil
		}
	}
	return restoreFromTrashOut{Path: dst}, nil
}

type labelsOut struct {
	Path   string   `json:"path"`
	Labels []string `json:"labels"`
}

func (c *call) labels(p string) ([]string, error) {
	res, err := c.gw.Stat(c.ctx, &provider.StatRequest{Ref: &provider.Reference{Path: p}, ArbitraryMetadataKeys: []string{"*"}})
	if err != nil {
		return nil, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("reva.labels.%s.", c.user.GetId().GetOpaqueId())
	ls := []string{}
	for k := range res.GetInfo().GetArbitraryMetadata().GetMetadata() {
		if l, ok := strings.CutPrefix(k, prefix); ok {
			ls = append(ls, l)
		}
	}
	slices.Sort(ls)
	return ls, nil
}

func (s *svc) getLabels(c *call, in pathIn) (labelsOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return labelsOut{}, err
	}
	c.path = p
	ls, err := c.labels(p)
	if err != nil {
		return labelsOut{}, err
	}
	return labelsOut{Path: p, Labels: ls}, nil
}

type setLabelsIn struct {
	Path   string   `json:"path" jsonschema:"absolute path of the file or folder"`
	Labels []string `json:"labels" jsonschema:"the labels the item must have"`
}

func (s *svc) setLabels(c *call, in setLabelsIn) (labelsOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return labelsOut{}, err
	}
	c.path = p
	current, err := c.labels(p)
	if err != nil {
		return labelsOut{}, err
	}
	ref := &provider.Reference{Path: p}
	if slices.Contains(in.Labels, "") {
		return labelsOut{}, newError(codeInvalidArgument, "a label cannot be empty")
	}
	for _, l := range in.Labels {
		if slices.Contains(current, l) {
			continue
		}
		res, err := c.gw.AddLabel(c.ctx, &labels.AddLabelRequest{Ref: ref, Label: l, UserId: c.user.GetId()})
		if err != nil {
			return labelsOut{}, asToolError(err, p)
		}
		if err := checkStatus(res.GetStatus(), "label "+l); err != nil {
			return labelsOut{}, err
		}
	}
	for _, l := range current {
		if slices.Contains(in.Labels, l) {
			continue
		}
		res, err := c.gw.RemoveLabel(c.ctx, &labels.RemoveLabelRequest{Ref: ref, Label: l, UserId: c.user.GetId()})
		if err != nil {
			return labelsOut{}, asToolError(err, p)
		}
		if err := checkStatus(res.GetStatus(), "label "+l); err != nil {
			return labelsOut{}, err
		}
	}
	ls, err := c.labels(p)
	if err != nil {
		return labelsOut{}, err
	}
	return labelsOut{Path: p, Labels: ls}, nil
}
