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
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

func (s *svc) doProppatch(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	//fn := r.URL.Path

	_, status, err := readProppatch(r.Body)
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(status)
		return
	}

	_, err = s.getClient()
	if err != nil {
		logger.Error(ctx, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNotImplemented)
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
				XMLName: xml.Name(t.(xml.StartElement).Name),
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
