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

package ocdav

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"go.opencensus.io/trace"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/pkg/errors"
)

func (s *svc) doProppatch(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "proppatch")
	defer span.End()
	log := appctx.GetLogger(ctx)

	fn := path.Join(ns, r.URL.Path)

	pp, status, err := readProppatch(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading proppatch")
		w.WriteHeader(status)
		return
	}

	c, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mkeys := []string{}

	pf := &propfindXML{
		Prop: propfindProps{},
	}
	rreq := &storageproviderv0alphapb.UnsetArbitraryMetadataRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
		ArbitraryMetadataKeys: []string{},
	}
	sreq := &storageproviderv0alphapb.SetArbitraryMetadataRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
		ArbitraryMetadata: &storageproviderv0alphapb.ArbitraryMetadata{
			Metadata: map[string]string{},
		},
	}
	for i := range pp {
		if len(pp[i].Props) < 1 {
			continue
		}
		for j := range pp[i].Props {
			pf.Prop = append(pf.Prop, pp[i].Props[j].XMLName)
			// don't use path.Join. It removes the double slash! concatenate with a /
			key := fmt.Sprintf("%s/%s", pp[i].Props[j].XMLName.Space, pp[i].Props[j].XMLName.Local)
			value := string(pp[i].Props[j].InnerXML)
			remove := pp[i].Remove
			// boolean flags may be "set" to false as well
			if s.isBooleanProperty(key) {
				// Make boolean properties either "0" or "1"
				value = s.as0or1(value)
				if value == "0" {
					remove = true
				}
			}
			if remove {
				rreq.ArbitraryMetadataKeys = append(rreq.ArbitraryMetadataKeys, key)
			} else {
				sreq.ArbitraryMetadata.Metadata[key] = value
			}
			mkeys = append(mkeys, key)
		}
		// what do we need to unset
		if len(rreq.ArbitraryMetadataKeys) > 0 {
			res, err := c.UnsetArbitraryMetadata(ctx, rreq)
			if err != nil {
				log.Error().Err(err).Msg("error sending a grpc UnsetArbitraryMetadata request")
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
		}
		if len(sreq.ArbitraryMetadata.Metadata) > 0 {
			res, err := c.SetArbitraryMetadata(ctx, sreq)
			if err != nil {
				log.Error().Err(err).Msg("error sending a grpc SetArbitraryMetadata request")
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
		}
	}

	req := &storageproviderv0alphapb.StatRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		},
		ArbitraryMetadataKeys: mkeys,
	}
	res, err := c.Stat(ctx, req)
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

	propRes, err := s.formatPropfind(ctx, pf, infos, ns)
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

func (s *svc) isBooleanProperty(prop string) bool {
	// TODO add other properties we know to be boolean?
	return prop == "http://owncloud.org/ns/favorite"
}

func (s *svc) as0or1(val string) string {
	switch strings.TrimSpace(val) {
	case "false":
		return "0"
	case "":
		return "0"
	case "0":
		return "0"
	case "no":
		return "0"
	case "off":
		return "0"
	}
	return "1"
}

// Proppatch describes a property update instruction as defined in RFC 4918.
// See http://www.webdav.org/specs/rfc4918.html#METHOD_PROPPATCH
type Proppatch struct {
	// Remove specifies whether this patch removes properties. If it does not
	// remove them, it sets them.
	Remove bool
	// Props contains the properties to be set or removed.
	Props []propertyXML
}

type xmlValue []byte

func (v *xmlValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// The XML value of a property can be arbitrary, mixed-content XML.
	// To make sure that the unmarshalled value contains all required
	// namespaces, we encode all the property value XML tokens into a
	// buffer. This forces the encoder to redeclare any used namespaces.
	var b bytes.Buffer
	e := xml.NewEncoder(&b)
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		if e, ok := t.(xml.EndElement); ok && e.Name == start.Name {
			break
		}
		if err = e.EncodeToken(t); err != nil {
			return err
		}
	}
	err := e.Flush()
	if err != nil {
		return err
	}
	*v = b.Bytes()
	return nil
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_prop (for proppatch)
type proppatchProps []propertyXML

// UnmarshalXML appends the property names and values enclosed within start
// to ps.
//
// An xml:lang attribute that is defined either on the DAV:prop or property
// name XML element is propagated to the property's Lang field.
//
// UnmarshalXML returns an error if start does not contain any properties or if
// property values contain syntactically incorrect XML.
func (ps *proppatchProps) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	lang := xmlLang(start, "")
	for {
		t, err := next(d)
		if err != nil {
			return err
		}
		switch elem := t.(type) {
		case xml.EndElement:
			if len(*ps) == 0 {
				return fmt.Errorf("%s must not be empty", start.Name.Local)
			}
			return nil
		case xml.StartElement:
			p := propertyXML{
				XMLName: t.(xml.StartElement).Name,
				Lang:    xmlLang(t.(xml.StartElement), lang),
			}
			err = d.DecodeElement(((*xmlValue)(&p.InnerXML)), &elem)
			if err != nil {
				return err
			}
			*ps = append(*ps, p)
		}
	}
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_set
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_remove
type setRemove struct {
	XMLName xml.Name
	Lang    string         `xml:"xml:lang,attr,omitempty"`
	Prop    proppatchProps `xml:"DAV: prop"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propertyupdate
type propertyupdate struct {
	XMLName   xml.Name    `xml:"DAV: propertyupdate"`
	Lang      string      `xml:"xml:lang,attr,omitempty"`
	SetRemove []setRemove `xml:",any"`
}

func readProppatch(r io.Reader) (patches []Proppatch, status int, err error) {
	var pu propertyupdate
	if err = xml.NewDecoder(r).Decode(&pu); err != nil {
		return nil, http.StatusBadRequest, err
	}
	for _, op := range pu.SetRemove {
		remove := false
		switch op.XMLName {
		case xml.Name{Space: "DAV:", Local: "set"}:
			// No-op.
		case xml.Name{Space: "DAV:", Local: "remove"}:
			for _, p := range op.Prop {
				if len(p.InnerXML) > 0 {
					return nil, http.StatusBadRequest, errInvalidProppatch
				}
			}
			remove = true
		default:
			return nil, http.StatusBadRequest, errInvalidProppatch
		}
		patches = append(patches, Proppatch{Remove: remove, Props: op.Prop})
	}
	return patches, 0, nil
}

var xmlLangName = xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "lang"}

func xmlLang(s xml.StartElement, d string) string {
	for _, attr := range s.Attr {
		if attr.Name == xmlLangName {
			return attr.Value
		}
	}
	return d
}

// Next returns the next token, if any, in the XML stream of d.
// RFC 4918 requires to ignore comments, processing instructions
// and directives.
// http://www.webdav.org/specs/rfc4918.html#property_values
// http://www.webdav.org/specs/rfc4918.html#xml-extensibility
func next(d *xml.Decoder) (xml.Token, error) {
	for {
		t, err := d.Token()
		if err != nil {
			return t, err
		}
		switch t.(type) {
		case xml.Comment, xml.Directive, xml.ProcInst:
			continue
		default:
			return t, nil
		}
	}
}

var errInvalidProppatch = errors.New("webdav: invalid proppatch")
