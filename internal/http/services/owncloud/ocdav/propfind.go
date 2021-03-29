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

package ocdav

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/trace"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxuser "github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
)

const (
	_nsDav      = "DAV:"
	_nsOwncloud = "http://owncloud.org/ns"
	_nsOCS      = "http://open-collaboration-services.org/ns"

	_propOcFavorite = "http://owncloud.org/ns/favorite"

	// RFC1123 time that mimics oc10. time.RFC1123 would end in "UTC", see https://github.com/golang/go/issues/13781
	RFC1123 = "Mon, 02 Jan 2006 15:04:05 GMT"

	// _propQuotaUncalculated = "-1"
	_propQuotaUnknown = "-2"
	// _propQuotaUnlimited    = "-3"
)

// ns is the namespace that is prefixed to the path in the cs3 namespace
func (s *svc) handlePropfind(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	// see https://tools.ietf.org/html/rfc4918#section-9.1
	if depth != "0" && depth != "1" && depth != "infinity" {
		sublog.Debug().Str("depth", depth).Msgf("invalid Depth header value")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metadataKeys := []string{}
	if pf.Allprop != nil {
		// TODO this changes the behavior and returns all properties if allprops has been set,
		// but allprops should only return some default properties
		// see https://tools.ietf.org/html/rfc4918#section-9.1
		// the description of arbitrary_metadata_keys in https://cs3org.github.io/cs3apis/#cs3.storage.provider.v1beta1.ListContainerRequest an others may need clarification
		// tracked in https://github.com/cs3org/cs3apis/issues/104
		metadataKeys = append(metadataKeys, "*")
	} else {
		for i := range pf.Prop {
			if requiresExplicitFetching(&pf.Prop[i]) {
				metadataKeys = append(metadataKeys, metadataKeyOf(&pf.Prop[i]))
			}
		}
	}
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	req := &provider.StatRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: metadataKeys,
	}
	res, err := client.Stat(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Interface("req", req).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	info := res.Info
	infos := []*provider.ResourceInfo{info}
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER && depth == "1" {
		req := &provider.ListContainerRequest{
			Ref:                   ref,
			ArbitraryMetadataKeys: metadataKeys,
		}
		res, err := client.ListContainer(ctx, req)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending list container grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(&sublog, w, res.Status)
			return
		}
		infos = append(infos, res.Infos...)
	} else if depth == "infinity" {
		// FIXME: doesn't work cross-storage as the results will have the wrong paths!
		// use a stack to explore sub-containers breadth-first
		stack := []string{info.Path}
		for len(stack) > 0 {
			// retrieve path on top of stack
			path := stack[len(stack)-1]
			ref = &provider.Reference{
				Spec: &provider.Reference_Path{Path: path},
			}
			req := &provider.ListContainerRequest{
				Ref:                   ref,
				ArbitraryMetadataKeys: metadataKeys,
			}
			res, err := client.ListContainer(ctx, req)
			if err != nil {
				sublog.Error().Err(err).Str("path", path).Msg("error sending list container grpc request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				HandleErrorStatus(&sublog, w, res.Status)
				return
			}

			infos = append(infos, res.Infos...)

			if depth != "infinity" {
				break
			}

			// TODO: stream response to avoid storing too many results in memory

			stack = stack[:len(stack)-1]

			// check sub-containers in reverse order and add them to the stack
			// the reversed order here will produce a more logical sorting of results
			for i := len(res.Infos) - 1; i >= 0; i-- {
				// for i := range res.Infos {
				if res.Infos[i].Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					stack = append(stack, res.Infos[i].Path)
				}
			}
		}
	}

	propRes, err := s.formatPropfind(ctx, &pf, infos, ns)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	var disableTus bool
	// let clients know this collection supports tus.io POST requests to start uploads
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if info.Opaque != nil {
			_, disableTus = info.Opaque.Map["disable_tus"]
		}
		if !disableTus {
			w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Tus-Version", "1.0.0")
			w.Header().Set("Tus-Extension", "creation,creation-with-upload")
		}
	}
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}

func requiresExplicitFetching(n *xml.Name) bool {
	switch n.Space {
	case _nsDav:
		switch n.Local {
		case "quota-available-bytes", "quota-used-bytes":
			//  A <DAV:allprop> PROPFIND request SHOULD NOT return DAV:quota-available-bytes and DAV:quota-used-bytes
			// from https://www.rfc-editor.org/rfc/rfc4331.html#section-2
			return true
		default:
			return false
		}
	case _nsOwncloud:
		switch n.Local {
		case "favorite", "share-types", "checksums", "size":
			return true
		default:
			return false
		}
	case _nsOCS:
		return false
	}
	return true
}

// from https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/xml.go#L178-L205
func readPropfind(r io.Reader) (pf propfindXML, status int, err error) {
	c := countingReader{r: r}
	if err = xml.NewDecoder(&c).Decode(&pf); err != nil {
		if err == io.EOF {
			if c.n == 0 {
				// An empty body means to propfind allprop.
				// http://www.webdav.org/specs/rfc4918.html#METHOD_PROPFIND
				return propfindXML{Allprop: new(struct{})}, 0, nil
			}
			err = errInvalidPropfind
		}
		return propfindXML{}, http.StatusBadRequest, err
	}

	if pf.Allprop == nil && pf.Include != nil {
		return propfindXML{}, http.StatusBadRequest, errInvalidPropfind
	}
	if pf.Allprop != nil && (pf.Prop != nil || pf.Propname != nil) {
		return propfindXML{}, http.StatusBadRequest, errInvalidPropfind
	}
	if pf.Prop != nil && pf.Propname != nil {
		return propfindXML{}, http.StatusBadRequest, errInvalidPropfind
	}
	if pf.Propname == nil && pf.Allprop == nil && pf.Prop == nil {
		// jfd: I think <d:prop></d:prop> is perfectly valid ... treat it as allprop
		return propfindXML{Allprop: new(struct{})}, 0, nil
	}
	return pf, 0, nil
}

func (s *svc) formatPropfind(ctx context.Context, pf *propfindXML, mds []*provider.ResourceInfo, ns string) (string, error) {
	responses := make([]*responseXML, 0, len(mds))
	for i := range mds {
		res, err := s.mdToPropResponse(ctx, pf, mds[i], ns)
		if err != nil {
			return "", err
		}
		responses = append(responses, res)
	}
	responsesXML, err := xml.Marshal(&responses)
	if err != nil {
		return "", err
	}

	msg := `<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:" `
	msg += `xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">`
	msg += string(responsesXML) + `</d:multistatus>`
	return msg, nil
}

func (s *svc) xmlEscaped(val string) []byte {
	buf := new(bytes.Buffer)
	xml.Escape(buf, []byte(val))
	return buf.Bytes()
}

func (s *svc) newPropNS(namespace string, local string, val string) *propertyXML {
	return &propertyXML{
		XMLName:  xml.Name{Space: namespace, Local: local},
		Lang:     "",
		InnerXML: s.xmlEscaped(val),
	}
}

// TODO properly use the space
func (s *svc) newProp(key, val string) *propertyXML {
	return &propertyXML{
		XMLName:  xml.Name{Space: "", Local: key},
		Lang:     "",
		InnerXML: s.xmlEscaped(val),
	}
}

// TODO properly use the space
func (s *svc) newPropRaw(key, val string) *propertyXML {
	return &propertyXML{
		XMLName:  xml.Name{Space: "", Local: key},
		Lang:     "",
		InnerXML: []byte(val),
	}
}

// mdToPropResponse converts the CS3 metadata into a webdav PropResponse
// ns is the CS3 namespace that needs to be removed from the CS3 path before
// prefixing it with the baseURI
func (s *svc) mdToPropResponse(ctx context.Context, pf *propfindXML, md *provider.ResourceInfo, ns string) (*responseXML, error) {
	sublog := appctx.GetLogger(ctx).With().Interface("md", md).Str("ns", ns).Logger()
	md.Path = strings.TrimPrefix(md.Path, ns)

	baseURI := ctx.Value(ctxKeyBaseURI).(string)

	ref := path.Join(baseURI, md.Path)
	if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := responseXML{
		Href:     encodePath(ref),
		Propstat: []propstatXML{},
	}

	var ls *link.PublicShare

	// -1 indicates uncalculated
	// -2 indicates unknown (default)
	// -3 indicates unlimited
	quota := _propQuotaUnknown
	size := fmt.Sprintf("%d", md.Size)
	// TODO refactor helper functions: GetOpaqueJSONEncoded(opaque, key string, *struct) err, GetOpaquePlainEncoded(opaque, key) value, err
	// or use ok like pattern and return bool?
	if md.Opaque != nil && md.Opaque.Map != nil {
		if md.Opaque.Map["link-share"] != nil && md.Opaque.Map["link-share"].Decoder == "json" {
			ls = &link.PublicShare{}
			err := json.Unmarshal(md.Opaque.Map["link-share"].Value, ls)
			if err != nil {
				sublog.Error().Err(err).Msg("could not unmarshal link json")
			}
		}
		if md.Opaque.Map["quota"] != nil && md.Opaque.Map["quota"].Decoder == "plain" {
			quota = string(md.Opaque.Map["quota"].Value)
		}
	}

	role := conversions.RoleFromResourcePermissions(md.PermissionSet)

	isShared := !isCurrentUserOwner(ctx, md.Owner)
	var wdp string
	isPublic := ls != nil
	if md.PermissionSet != nil {
		wdp = role.WebDAVPermissions(
			md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			isShared,
			false,
			isPublic,
		)
		sublog.Debug().Interface("role", role).Str("dav-permissions", wdp).Msg("converted PermissionSet")
	}

	propstatOK := propstatXML{
		Status: "HTTP/1.1 200 OK",
		Prop:   []*propertyXML{},
	}
	propstatNotFound := propstatXML{
		Status: "HTTP/1.1 404 Not Found",
		Prop:   []*propertyXML{},
	}
	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties

		if md.Id != nil {
			id := wrapResourceID(md.Id)
			propstatOK.Prop = append(propstatOK.Prop,
				s.newProp("oc:id", id),
				s.newProp("oc:fileid", id),
			)
		}

		if md.Etag != "" {
			// etags must be enclosed in double quotes and cannot contain them.
			// See https://tools.ietf.org/html/rfc7232#section-2.3 for details
			// TODO(jfd) handle weak tags that start with 'W/'
			propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getetag", md.Etag))
		}

		if md.PermissionSet != nil {
			propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:permissions", wdp))
		}

		// always return size, well nearly always ... public link shares are a little weird
		if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			propstatOK.Prop = append(propstatOK.Prop, s.newPropRaw("d:resourcetype", "<d:collection/>"))
			if ls == nil {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:size", size))
			}
			// A <DAV:allprop> PROPFIND request SHOULD NOT return DAV:quota-available-bytes and DAV:quota-used-bytes
			// from https://www.rfc-editor.org/rfc/rfc4331.html#section-2
			// propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:quota-used-bytes", size))
			// propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:quota-available-bytes", quota))
		} else {
			propstatOK.Prop = append(propstatOK.Prop,
				s.newProp("d:resourcetype", ""),
				s.newProp("d:getcontentlength", size),
			)
			if md.MimeType != "" {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", md.MimeType))
			}
		}
		// Finder needs the getLastModified property to work.
		if md.Mtime != nil {
			t := utils.TSToTime(md.Mtime).UTC()
			lastModifiedString := t.Format(RFC1123)
			propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getlastmodified", lastModifiedString))
		}

		// stay bug compatible with oc10, see https://github.com/owncloud/core/pull/38304#issuecomment-762185241
		var checksums strings.Builder
		if md.Checksum != nil {
			checksums.WriteString("<oc:checksum>")
			checksums.WriteString(strings.ToUpper(string(storageprovider.GRPC2PKGXS(md.Checksum.Type))))
			checksums.WriteString(":")
			checksums.WriteString(md.Checksum.Sum)
		}
		if md.Opaque != nil {
			if e, ok := md.Opaque.Map["md5"]; ok {
				if checksums.Len() == 0 {
					checksums.WriteString("<oc:checksum>MD5:")
				} else {
					checksums.WriteString(" MD5:")
				}
				checksums.WriteString(string(e.Value))
			}
			if e, ok := md.Opaque.Map["adler32"]; ok {
				if checksums.Len() == 0 {
					checksums.WriteString("<oc:checksum>ADLER32:")
				} else {
					checksums.WriteString(" ADLER32:")
				}
				checksums.WriteString(string(e.Value))
			}
		}
		if checksums.Len() > 0 {
			checksums.WriteString("</oc:checksum>")
			propstatOK.Prop = append(propstatOK.Prop, s.newPropRaw("oc:checksums", checksums.String()))
		}

		// ls do not report any properties as missing by default
		if ls == nil {
			// favorites from arbitrary metadata
			if k := md.GetArbitraryMetadata(); k == nil {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
			} else if amd := k.GetMetadata(); amd == nil {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
			} else if v, ok := amd[_propOcFavorite]; ok && v != "" {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", v))
			} else {
				propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
			}
		}
		// TODO return other properties ... but how do we put them in a namespace?
	} else {
		// otherwise return only the requested properties
		for i := range pf.Prop {
			switch pf.Prop[i].Space {
			case _nsOwncloud:
				switch pf.Prop[i].Local {
				// TODO(jfd): maybe phoenix and the other clients can just use this id as an opaque string?
				// I tested the desktop client and phoenix to annotate which properties are requestted, see below cases
				case "fileid": // phoenix only
					if md.Id != nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:fileid", wrapResourceID(md.Id)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:fileid", ""))
					}
				case "id": // desktop client only
					if md.Id != nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:id", wrapResourceID(md.Id)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:id", ""))
					}
				case "permissions": // both
					// oc:permissions take several char flags to indicate the permissions the user has on this node:
					// D = delete
					// NV = update (renameable moveable)
					// W = update (files only)
					// CK = create (folders only)
					// S = Shared
					// R = Shareable (Reshare)
					// M = Mounted
					// in contrast, the ocs:share-permissions further down below indicate clients the maximum permissions that can be granted
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:permissions", wdp))
				case "public-link-permission": // only on a share root node
					if ls != nil && md.PermissionSet != nil {
						propstatOK.Prop = append(
							propstatOK.Prop,
							s.newProp("oc:public-link-permission", strconv.FormatUint(uint64(role.OCSPermissions()), 10)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-permission", ""))
					}
				case "public-link-item-type": // only on a share root node
					if ls != nil {
						if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-item-type", "folder"))
						} else {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-item-type", "file"))
							// redirectref is another option
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-item-type", ""))
					}
				case "public-link-share-datetime":
					if ls != nil && ls.Mtime != nil {
						t := utils.TSToTime(ls.Mtime).UTC() // TODO or ctime?
						shareTimeString := t.Format(RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-share-datetime", shareTimeString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-share-datetime", ""))
					}
				case "public-link-share-owner":
					if ls != nil && ls.Owner != nil {
						if isCurrentUserOwner(ctx, ls.Owner) {
							u := ctxuser.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-share-owner", u.Username))
						} else {
							u, _ := ctxuser.ContextGetUser(ctx)
							sublog.Error().Interface("share", ls).Interface("user", u).Msg("the current user in the context should be the owner of a public link share")
							propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-share-owner", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-share-owner", ""))
					}
				case "public-link-expiration":
					if ls != nil && ls.Expiration != nil {
						t := utils.TSToTime(ls.Expiration).UTC()
						expireTimeString := t.Format(RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-expiration", expireTimeString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-expiration", ""))
					}
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-expiration", ""))
				case "size": // phoenix only
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					// TODO what is the difference to d:quota-used-bytes (which only exists for collections)?
					// oc:size is available on files and folders and behaves like d:getcontentlength or d:quota-used-bytes respectively
					if ls == nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:size", size))
					} else {
						// link share root collection has no size
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:size", ""))
					}
				case "owner-id": // phoenix only
					if md.Owner != nil {
						if isCurrentUserOwner(ctx, md.Owner) {
							u := ctxuser.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:owner-id", u.Username))
						} else {
							sublog.Debug().Msg("TODO fetch user username")
							propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-id", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-id", ""))
					}
				case "favorite": // phoenix only
					// TODO: can be 0 or 1?, in oc10 it is present or not
					// TODO: read favorite via separate call? that would be expensive? I hope it is in the md
					// TODO: this boolean favorite property is so horribly wrong ... either it is presont, or it is not ... unless ... it is possible to have a non binary value ... we need to double check
					if ls == nil {
						if k := md.GetArbitraryMetadata(); k == nil {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
						} else if amd := k.GetMetadata(); amd == nil {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
						} else if v, ok := amd[_propOcFavorite]; ok && v != "" {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "1"))
						} else {
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
						}
					} else {
						// link share root collection has no favorite
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:favorite", ""))
					}
				case "checksums": // desktop ... not really ... the desktop sends the OC-Checksum header

					// stay bug compatible with oc10, see https://github.com/owncloud/core/pull/38304#issuecomment-762185241
					var checksums strings.Builder
					if md.Checksum != nil {
						checksums.WriteString("<oc:checksum>")
						checksums.WriteString(strings.ToUpper(string(storageprovider.GRPC2PKGXS(md.Checksum.Type))))
						checksums.WriteString(":")
						checksums.WriteString(md.Checksum.Sum)
					}
					if md.Opaque != nil {
						if e, ok := md.Opaque.Map["md5"]; ok {
							if checksums.Len() == 0 {
								checksums.WriteString("<oc:checksum>MD5:")
							} else {
								checksums.WriteString(" MD5:")
							}
							checksums.WriteString(string(e.Value))
						}
						if e, ok := md.Opaque.Map["adler32"]; ok {
							if checksums.Len() == 0 {
								checksums.WriteString("<oc:checksum>ADLER32:")
							} else {
								checksums.WriteString(" ADLER32:")
							}
							checksums.WriteString(string(e.Value))
						}
					}
					if checksums.Len() > 13 {
						checksums.WriteString("</oc:checksum>")
						propstatOK.Prop = append(propstatOK.Prop, s.newPropRaw("oc:checksums", checksums.String()))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:checksums", ""))
					}
				case "share-types": // desktop
					k := md.GetArbitraryMetadata()
					amd := k.GetMetadata()
					if amdv, ok := amd[metadataKeyOf(&pf.Prop[i])]; ok {
						st := fmt.Sprintf("<oc:share-type>%s</oc:share-type>", amdv)
						propstatOK.Prop = append(propstatOK.Prop, s.newPropRaw("oc:share-types", st))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
					}
				case "owner-display-name": // phoenix only
					if md.Owner != nil {
						if isCurrentUserOwner(ctx, md.Owner) {
							u := ctxuser.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:owner-display-name", u.DisplayName))
						} else {
							sublog.Debug().Msg("TODO fetch user displayname")
							propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-display-name", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-display-name", ""))
					}
				case "downloadURL": // desktop
					if isPublic && md.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
						var path string
						if !ls.PasswordProtected {
							path = md.Path
						} else {
							expiration := time.Unix(int64(ls.Signature.SignatureExpiration.Seconds), int64(ls.Signature.SignatureExpiration.Nanos))
							var sb strings.Builder

							sb.WriteString(md.Path)
							sb.WriteString("?signature=")
							sb.WriteString(ls.Signature.Signature)
							sb.WriteString("&expiration=")
							sb.WriteString(url.QueryEscape(expiration.Format(time.RFC3339)))

							path = sb.String()
						}
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:downloadURL", s.c.PublicURL+baseURI+path))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
					}
				case "privatelink": // phoenix only
					// <oc:privatelink>https://phoenix.owncloud.com/f/9</oc:privatelink>
					fallthrough
				case "dDC": // desktop
					fallthrough
				case "data-fingerprint": // desktop
					// used by admins to indicate a backup has been restored,
					// can only occur on the root node
					// server implementation in https://github.com/owncloud/core/pull/24054
					// see https://doc.owncloud.com/server/admin_manual/configuration/server/occ_command.html#maintenance-commands
					// TODO(jfd): double check the client behavior with reva on backup restore
					fallthrough
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
				}
			case _nsDav:
				switch pf.Prop[i].Local {
				case "getetag": // both
					if md.Etag != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getetag", md.Etag))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getetag", ""))
					}
				case "getcontentlength": // both
					// see everts stance on this https://stackoverflow.com/a/31621912, he points to http://tools.ietf.org/html/rfc4918#section-15.3
					// > Purpose: Contains the Content-Length header returned by a GET without accept headers.
					// which only would make sense when eg. rendering a plain HTML filelisting when GETing a collection,
					// which is not the case ... so we don't return it on collections. owncloud has oc:size for that
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getcontentlength", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
					}
				case "resourcetype": // both
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newPropRaw("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype": // phoenix
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// directories have no contenttype
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getcontenttype", ""))
					} else if md.MimeType != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", md.MimeType))
					}
				case "getlastmodified": // both
					// TODO we cannot find out if md.Mtime is set or not because ints in go default to 0
					if md.Mtime != nil {
						t := utils.TSToTime(md.Mtime).UTC()
						lastModifiedString := t.Format(RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getlastmodified", lastModifiedString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getlastmodified", ""))
					}
				case "quota-used-bytes": // RFC 4331
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// always returns the current usage,
						// in oc10 there seems to be a bug that makes the size in webdav differ from the one in the user properties, not taking shares into account
						// in ocis we plan to always mak the quota a property of the storage space
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:quota-used-bytes", size))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:quota-used-bytes", ""))
					}
				case "quota-available-bytes": // RFC 4331
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// oc10 returns -3 for unlimited, -2 for unknown, -1 for uncalculated
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:quota-available-bytes", quota))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:quota-available-bytes", ""))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:"+pf.Prop[i].Local, ""))
				}
			case _nsOCS:
				switch pf.Prop[i].Local {
				// ocs:share-permissions indicate clients the maximum permissions that can be granted:
				// 1 = read
				// 2 = write (update)
				// 4 = create
				// 8 = delete
				// 16 = share
				// shared files can never have the create or delete permission bit set
				case "share-permissions":
					if md.PermissionSet != nil {
						perms := role.OCSPermissions()
						// shared files cant have the create or delete permission set
						if md.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
							perms &^= conversions.PermissionCreate
							perms &^= conversions.PermissionDelete
						}
						propstatOK.Prop = append(propstatOK.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, strconv.FormatUint(uint64(perms), 10)))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:"+pf.Prop[i].Local, ""))
				}
			default:
				// handle custom properties
				if k := md.GetArbitraryMetadata(); k == nil {
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				} else if amd := k.GetMetadata(); amd == nil {
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				} else if v, ok := amd[metadataKeyOf(&pf.Prop[i])]; ok && v != "" {
					propstatOK.Prop = append(propstatOK.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, v))
				} else {
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				}
			}
		}
	}

	if len(propstatOK.Prop) > 0 {
		response.Propstat = append(response.Propstat, propstatOK)
	}
	if len(propstatNotFound.Prop) > 0 {
		response.Propstat = append(response.Propstat, propstatNotFound)
	}

	return &response, nil
}

// a file is only yours if you are the owner
func isCurrentUserOwner(ctx context.Context, owner *userv1beta1.UserId) bool {
	contextUser, ok := ctxuser.ContextGetUser(ctx)
	if ok && contextUser.Id != nil && owner != nil &&
		contextUser.Id.Idp == owner.Idp &&
		contextUser.Id.OpaqueId == owner.OpaqueId {
		return true
	}
	return false
}

type countingReader struct {
	n int
	r io.Reader
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func metadataKeyOf(n *xml.Name) string {
	switch {
	case n.Space == _nsDav && n.Local == "quota-available-bytes":
		return "quota"
	default:
		return fmt.Sprintf("%s/%s", n.Space, n.Local)
	}
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_prop (for propfind)
type propfindProps []xml.Name

// UnmarshalXML appends the property names enclosed within start to pn.
//
// It returns an error if start does not contain any properties or if
// properties contain values. Character data between properties is ignored.
func (pn *propfindProps) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		switch e := t.(type) {
		case xml.EndElement:
			// jfd: I think <d:prop></d:prop> is perfectly valid ... treat it as allprop
			/*
				if len(*pn) == 0 {
					return fmt.Errorf("%s must not be empty", start.Name.Local)
				}
			*/
			return nil
		case xml.StartElement:
			t, err = next(d)
			if err != nil {
				return err
			}
			if _, ok := t.(xml.EndElement); !ok {
				return fmt.Errorf("unexpected token %T", t)
			}
			*pn = append(*pn, e.Name)
		}
	}
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propfind
type propfindXML struct {
	XMLName  xml.Name      `xml:"DAV: propfind"`
	Allprop  *struct{}     `xml:"DAV: allprop"`
	Propname *struct{}     `xml:"DAV: propname"`
	Prop     propfindProps `xml:"DAV: prop"`
	Include  propfindProps `xml:"DAV: include"`
}

type responseXML struct {
	XMLName             xml.Name      `xml:"d:response"`
	Href                string        `xml:"d:href"`
	Propstat            []propstatXML `xml:"d:propstat"`
	Status              string        `xml:"d:status,omitempty"`
	Error               *errorXML     `xml:"d:error"`
	ResponseDescription string        `xml:"d:responsedescription,omitempty"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propstat
type propstatXML struct {
	// Prop requires DAV: to be the default namespace in the enclosing
	// XML. This is due to the standard encoding/xml package currently
	// not honoring namespace declarations inside a xmltag with a
	// parent element for anonymous slice elements.
	// Use of multistatusWriter takes care of this.
	Prop                []*propertyXML `xml:"d:prop>_ignored_"`
	Status              string         `xml:"d:status"`
	Error               *errorXML      `xml:"d:error"`
	ResponseDescription string         `xml:"d:responsedescription,omitempty"`
}

// Property represents a single DAV resource property as defined in RFC 4918.
// http://www.webdav.org/specs/rfc4918.html#data.model.for.resource.properties
type propertyXML struct {
	// XMLName is the fully qualified name that identifies this property.
	XMLName xml.Name

	// Lang is an optional xml:lang attribute.
	Lang string `xml:"xml:lang,attr,omitempty"`

	// InnerXML contains the XML representation of the property value.
	// See http://www.webdav.org/specs/rfc4918.html#property_values
	//
	// Property values of complex type or mixed-content must have fully
	// expanded XML namespaces or be self-contained with according
	// XML namespace declarations. They must not rely on any XML
	// namespace declarations within the scope of the XML document,
	// even including the DAV: namespace.
	InnerXML []byte `xml:",innerxml"`
}
