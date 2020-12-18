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
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxuser "github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/pkg/errors"
)

// ns is the namespace that is prefixed to the path in the cs3 namespace
func (s *svc) handlePropfind(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()

	ns = applyLayout(ctx, ns)

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

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	req := &provider.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msgf("error sending a grpc stat request to ref: %v", ref)
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
			Ref: ref,
			ArbitraryMetadataKeys: []string{
				"http://owncloud.org/ns/share-types",
			},
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
				Ref: ref,
				ArbitraryMetadataKeys: []string{
					"http://owncloud.org/ns/share-types",
				},
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
				//for i := range res.Infos {
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
	// let clients know this collection supports tus.io POST requests to start uploads
	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.Header().Set("Tus-Version", "1.0.0")
		w.Header().Set("Tus-Extension", "creation,creation-with-upload")
	}
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
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

func (s *svc) xmlEscaped(val string) string {
	buf := new(bytes.Buffer)
	xml.Escape(buf, []byte(val))
	return buf.String()
}

func (s *svc) newPropNS(namespace string, local string, val string) *propertyXML {
	return &propertyXML{
		XMLName:  xml.Name{Space: namespace, Local: local},
		Lang:     "",
		InnerXML: []byte(val),
	}
}

func (s *svc) newProp(key, val string) *propertyXML {
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
		Href:     (&url.URL{Path: ref}).EscapedPath(), // url encode response.Href
		Propstat: []propstatXML{},
	}

	// files never have the create container permission
	if md.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		md.PermissionSet = copyPermissionSet(md.PermissionSet)
		md.PermissionSet.CreateContainer = false
	}

	var ls *link.PublicShare
	if md.Opaque != nil && md.Opaque.Map != nil && md.Opaque.Map["link-share"] != nil && md.Opaque.Map["link-share"].Decoder == "json" {
		ls = &link.PublicShare{}
		err := json.Unmarshal(md.Opaque.Map["link-share"].Value, ls)
		if err != nil {
			sublog.Error().Err(err).Msg("could not unmarshal link json")
		}
		// shared files never have the delete permission
		if md.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
			// the permission set has already been copied above
			md.PermissionSet.Delete = false
		}
	}

	isShared := !isCurrentUserOwner(ctx, md.Owner)

	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties
		response.Propstat = append(response.Propstat, propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		})

		if md.Id != nil {
			id := wrapResourceID(md.Id)
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				s.newProp("oc:id", id),
				s.newProp("oc:fileid", id),
			)
		}

		if md.Etag != "" {
			// etags must be enclosed in double quotes and cannot contain them.
			// See https://tools.ietf.org/html/rfc7232#section-2.3 for details
			// TODO(jfd) handle weak tags that start with 'W/'
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:getetag", md.Etag))
		}

		if md.PermissionSet != nil {
			r := conversions.RoleFromResourcePermissions(md.PermissionSet)
			wdp := r.WebDAVPermissions(
				md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				isShared,
				false,
				false,
			)
			sublog.Debug().Interface("role", r).Str("dav-permissions", wdp).Msg("converted PermissionSet")
			response.Propstat[0].Prop = append(
				response.Propstat[0].Prop,
				s.newProp("oc:permissions", wdp))
		}

		// always return size
		size := fmt.Sprintf("%d", md.Size)
		if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				s.newProp("d:resourcetype", "<d:collection/>"),
				s.newProp("d:getcontenttype", "httpd/unix-directory"),
				s.newProp("oc:size", size),
			)
		} else {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				s.newProp("d:getcontentlength", size),
			)
			if md.MimeType != "" {
				response.Propstat[0].Prop = append(response.Propstat[0].Prop,
					s.newProp("d:getcontenttype", md.MimeType),
				)
			}
		}
		// Finder needs the getLastModified property to work.
		t := utils.TSToTime(md.Mtime).UTC()
		lastModifiedString := t.Format(time.RFC1123Z)
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:getlastmodified", lastModifiedString))

		if md.Checksum != nil {
			// TODO(jfd): the actual value is an abomination like this:
			// <oc:checksums>
			//   <oc:checksum>SHA1:9bd253a09d58be107bcb4169ebf338c8df34d086 MD5:d90bcc6bf847403d22a4abba64e79994 ADLER32:fca23ff5</oc:checksum>
			// </oc:checksums>
			// yep, correct, space delimited key value pairs inside an oc:checksum tag inside an oc:checksums tag
			value := fmt.Sprintf("<oc:checksum>%s:%s</oc:checksum>", md.Checksum.Type, md.Checksum.Sum)
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:checksums", value))
		}

		// favorites from arbitrary metadata
		if k := md.GetArbitraryMetadata(); k == nil {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:favorite", "0"))
		} else if amd := k.GetMetadata(); amd == nil {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:favorite", "0"))
		} else if v, ok := amd["http://owncloud.org/ns/favorite"]; ok && v != "" {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:favorite", "1"))
		} else {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:favorite", "0"))
		}
		// TODO return other properties ... but how do we put them in a namespace?
	} else {
		// otherwise return only the requested properties
		propstatOK := propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		}
		propstatNotFound := propstatXML{
			Status: "HTTP/1.1 404 Not Found",
			Prop:   []*propertyXML{},
		}
		size := fmt.Sprintf("%d", md.Size)
		for i := range pf.Prop {
			switch pf.Prop[i].Space {
			case "http://owncloud.org/ns":
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
					if md.PermissionSet != nil {
						r := conversions.RoleFromResourcePermissions(md.PermissionSet)
						wdp := r.WebDAVPermissions(
							md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
							isShared,
							false,
							false,
						)
						sublog.Debug().Interface("role", r).Str("dav-permissions", wdp).Msg("converted PermissionSet")
						propstatOK.Prop = append(
							propstatOK.Prop,
							s.newProp("oc:permissions", wdp))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:permissions", ""))
					}
				case "public-link-permission": // only on a share root node
					if ls != nil && md.PermissionSet != nil {
						r := conversions.RoleFromResourcePermissions(md.PermissionSet)
						propstatOK.Prop = append(
							propstatOK.Prop,
							s.newProp("oc:public-link-permission", strconv.FormatUint(uint64(r.OCSPermissions()), 10)))
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
						shareTimeString := t.Format(time.RFC1123Z)
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
						expireTimeString := t.Format(time.RFC1123Z)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:public-link-expiration", expireTimeString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-expiration", ""))
					}
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:public-link-expiration", ""))
				case "size": // phoenix only
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					// oc:size is also available on folders
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:size", size))
				case "owner-id": // phoenix only
					if md.Owner != nil && md.Owner.OpaqueId != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:owner-id", s.xmlEscaped(md.Owner.OpaqueId)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-id", ""))
					}
				case "favorite": // phoenix only
					// TODO: can be 0 or 1?, in oc10 it is present or not
					// TODO: read favorite via separate call? that would be expensive? I hope it is in the md
					// TODO: this boolean favorite property is so horribly wrong ... either it is presont, or it is not ... unless ... it is possible to have a non binary value ... we need to double check
					if k := md.GetArbitraryMetadata(); k == nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
					} else if amd := k.GetMetadata(); amd == nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
					} else if v, ok := amd["http://owncloud.org/ns/favorite"]; ok && v != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "1"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:favorite", "0"))
					}
				case "checksums": // desktop
					if md.Checksum != nil {
						// TODO(jfd): the actual value is an abomination like this:
						// <oc:checksums>
						//   <oc:checksum>SHA1:9bd253a09d58be107bcb4169ebf338c8df34d086 MD5:d90bcc6bf847403d22a4abba64e79994 ADLER32:fca23ff5</oc:checksum>
						// </oc:checksums>
						// yep, correct, space delimited key value pairs inside an oc:checksum tag inside an oc:checksums tag
						value := fmt.Sprintf("<oc:checksum>%s:%s</oc:checksum>", md.Checksum.Type, md.Checksum.Sum)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:checksums", value))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:checksums", ""))
					}
				case "share-types": // desktop
					k := md.GetArbitraryMetadata()
					amd := k.GetMetadata()
					if amdv, ok := amd[fmt.Sprintf("%s/%s", pf.Prop[i].Space, pf.Prop[i].Local)]; ok {
						st := fmt.Sprintf("<oc:share-type>%s</oc:share-type>", amdv)
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:share-types", st))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
					}
				case "owner-display-name": // phoenix only
					if md.Owner != nil {
						if isCurrentUserOwner(ctx, md.Owner) {
							u := ctxuser.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:owner-display-name", u.Username))
						} else {
							sublog.Debug().Msg("TODO fetch user displayname")
							propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-display-name", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:owner-display-name", ""))
					}
				case "privatelink": // phoenix only
					// <oc:privatelink>https://phoenix.owncloud.com/f/9</oc:privatelink>
					fallthrough
				case "downloadUrl": // desktop
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
			case "DAV:":
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
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype": // phoenix
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", "httpd/unix-directory"))
					} else if md.MimeType != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", md.MimeType))
					}
				case "getlastmodified": // both
					// TODO we cannot find out if md.Mtime is set or not because ints in go default to 0
					t := utils.TSToTime(md.Mtime).UTC()
					lastModifiedString := t.Format(time.RFC1123Z)
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getlastmodified", lastModifiedString))
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:"+pf.Prop[i].Local, ""))
				}
			case "http://open-collaboration-services.org/ns":
				switch pf.Prop[i].Local {
				case "share-permissions":
					if md.PermissionSet != nil {
						perms := conversions.RoleFromResourcePermissions(md.PermissionSet).OCSPermissions()
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
				} else if v, ok := amd[fmt.Sprintf("%s/%s", pf.Prop[i].Space, pf.Prop[i].Local)]; ok && v != "" {
					propstatOK.Prop = append(propstatOK.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, v))
				} else {
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				}
			}
		}
		if len(propstatOK.Prop) > 0 {
			response.Propstat = append(response.Propstat, propstatOK)
		}
		if len(propstatNotFound.Prop) > 0 {
			response.Propstat = append(response.Propstat, propstatNotFound)
		}
	}

	return &response, nil
}

// the permission set that is passed in needs to be copied so we don't accidentially overwrite the permission set of other entries
func copyPermissionSet(p *provider.ResourcePermissions) *provider.ResourcePermissions {
	if p == nil {
		return nil
	}
	return &provider.ResourcePermissions{
		AddGrant:             p.AddGrant,
		CreateContainer:      p.CreateContainer,
		Delete:               p.Delete,
		GetPath:              p.GetPath,
		GetQuota:             p.GetQuota,
		InitiateFileDownload: p.InitiateFileDownload,
		InitiateFileUpload:   p.InitiateFileUpload,
		ListGrants:           p.ListGrants,
		ListContainer:        p.ListContainer,
		ListFileVersions:     p.ListFileVersions,
		ListRecycle:          p.ListRecycle,
		Move:                 p.Move,
		RemoveGrant:          p.RemoveGrant,
		PurgeRecycle:         p.PurgeRecycle,
		RestoreFileVersion:   p.RestoreFileVersion,
		RestoreRecycleItem:   p.RestoreRecycleItem,
		Stat:                 p.Stat,
		UpdateGrant:          p.UpdateGrant,
	}
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
	// See http://www.ocwebdav.org/specs/rfc4918.html#property_values
	//
	// Property values of complex type or mixed-content must have fully
	// expanded XML namespaces or be self-contained with according
	// XML namespace declarations. They must not rely on any XML
	// namespace declarations within the scope of the XML document,
	// even including the DAV: namespace.
	InnerXML []byte `xml:",innerxml"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_error
type errorXML struct {
	XMLName   xml.Name `xml:"d:error"`
	Xmlnsd    string   `xml:"xmlns:d,attr"`
	Xmlnss    string   `xml:"xmlns:s,attr"`
	Exception string   `xml:"s:exception"`
	Message   string   `xml:"s:message"`
	InnerXML  []byte   `xml:",innerxml"`
}

var errInvalidPropfind = errors.New("webdav: invalid propfind")
