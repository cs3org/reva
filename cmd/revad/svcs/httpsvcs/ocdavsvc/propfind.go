// Copyright 2018-2019 CERN
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

package ocdavsvc

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.opencensus.io/trace"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs/utils"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
)

func (s *svc) doPropfind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()
	log := appctx.GetLogger(ctx)

	fn := r.URL.Path
	listChildren := r.Header.Get("Depth") != "0"

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
	}
	req := &storageproviderv0alphapb.StatRequest{Ref: ref}
	res, err := client.Stat(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			log.Warn().Str("path", fn).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	info := res.Info
	infos := []*storageproviderv0alphapb.ResourceInfo{info}
	if info.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER && listChildren {
		req := &storageproviderv0alphapb.ListContainerRequest{
			Ref: ref,
		}
		res, err := client.ListContainer(ctx, req)
		if err != nil {
			log.Error().Err(err).Msg("error sending list container grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if res.Status.Code != rpcpb.Code_CODE_OK {
			log.Err(err).Msg("error calling grpc list container")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		infos = append(infos, res.Infos...)
	}

	propRes, err := s.formatPropfind(ctx, &pf, infos)
	if err != nil {
		log.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		log.Err(err).Msg("error writing response")
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

func (s *svc) formatPropfind(ctx context.Context, pf *propfindXML, mds []*storageproviderv0alphapb.ResourceInfo) (string, error) {
	responses := make([]*responseXML, 0, len(mds))
	for i := range mds {
		res, err := s.mdToPropResponse(ctx, pf, mds[i])
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

func (s *svc) newProp(key, val string) *propertyXML {
	return &propertyXML{
		XMLName:  xml.Name{Space: "", Local: key},
		Lang:     "",
		InnerXML: []byte(val),
	}
}

func (s *svc) mdToPropResponse(ctx context.Context, pf *propfindXML, md *storageproviderv0alphapb.ResourceInfo) (*responseXML, error) {

	baseURI := ctx.Value(ctxKeyBaseURI).(string)
	// the old webdav endpoint does not contain the username
	if strings.HasPrefix(baseURI, "/remote.php/webdav") {
		// remove username from filename
		u, ok := user.ContextGetUser(ctx)
		if !ok {
			err := errors.Wrap(errtypes.UserRequired("userrequired"), "error getting user from ctx")
			return nil, err
		}
		// TODO can lead to slice out of bounds
		md.Path = md.Path[len(u.Username)+1:]
	}

	ref := path.Join(baseURI, md.Path)
	if md.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := responseXML{
		Href:     (&url.URL{Path: ref}).EscapedPath(), // url encode response.Href
		Propstat: []propstatXML{},
	}

	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties
		response.Propstat = append(response.Propstat, propstatXML{
			Status: "HTTP/1.1 200 OK",
			Prop:   []*propertyXML{},
		})
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:fileid", wrapResourceID(md.Id)))
		if md.Etag != "" {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:getetag", md.Etag))
		}
		if md.PermissionSet != nil {
			// TODO(jfd) no longer hardcode permissions
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("oc:permissions", "WCKDNVR"))
		}
		// always return size
		size := fmt.Sprintf("%d", md.Size)
		response.Propstat[0].Prop = append(response.Propstat[0].Prop,
			s.newProp("d:getcontentlength", size),
			s.newProp("oc:size", size),
		)
		if md.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop,
				s.newProp("d:resourcetype", "<d:collection/>"),
				s.newProp("d:getcontenttype", "httpd/unix-directory"),
			)
		} else if md.MimeType != "" {
			response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:getcontenttype", md.MimeType))
		}
		// Finder needs the the getLastModified property to work.
		t := utils.TSToTime(md.Mtime).UTC()
		lasModifiedString := t.Format(time.RFC1123)
		response.Propstat[0].Prop = append(response.Propstat[0].Prop, s.newProp("d:getlastmodified", lasModifiedString))

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
				case "fileid": // TODO upper lowercase sensivity?
					if md.Id != nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:fileid", wrapResourceID(md.Id)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:fileid", ""))
					}
				case "permissions":
					if md.PermissionSet != nil {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:permissions", "WCKDNVR"))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:permissions", ""))
					}
				case "size":
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("oc:size", size))
				case "favorite":
					fallthrough
				case "owner-id":
					fallthrough
				case "owner-display-name":
					fallthrough
				case "privatelink":
					fallthrough
				case "downloadUrl":
					fallthrough
				case "dDC":
					fallthrough
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("oc:"+pf.Prop[i].Local, ""))
				}
			case "DAV:":
				switch pf.Prop[i].Local {
				case "getetag":
					if md.Etag != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getetag", md.Etag))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:getetag", ""))
					}
				case "getcontentlength":
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontentlength", size))
				case "resourcetype":
					if md.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype":
					if md.Type == storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", "httpd/unix-directory"))
					} else if md.MimeType != "" {
						propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getcontenttype", md.MimeType))
					}
				case "getlastmodified":
					// TODO we cannot find out if md.Mtime is set or not because ints in go default to 0
					t := utils.TSToTime(md.Mtime).UTC()
					lastModifiedString := t.Format(time.RFC1123)
					propstatOK.Prop = append(propstatOK.Prop, s.newProp("d:getlastmodified", lastModifiedString))
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp("d:"+pf.Prop[i].Local, ""))
				}
			default:
				// TODO (jfd) lookup shortname for unknown namespaces?
				propstatNotFound.Prop = append(propstatNotFound.Prop, s.newProp(pf.Prop[i].Space+":"+pf.Prop[i].Local, ""))
			}
		}
		response.Propstat = append(response.Propstat, propstatOK, propstatNotFound)
	}

	return &response, nil
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
	XMLName  xml.Name `xml:"d:error"`
	InnerXML []byte   `xml:",innerxml"`
}

var errInvalidPropfind = errors.New("webdav: invalid propfind")
