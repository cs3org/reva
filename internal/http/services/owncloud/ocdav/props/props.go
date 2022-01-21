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

package props

import (
	"bytes"
	"encoding/xml"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// PropertyXML represents a single DAV resource property as defined in RFC 4918.
// http://www.webdav.org/specs/rfc4918.html#data.model.for.resource.properties
type PropertyXML struct {
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

func xmlEscaped(val string) []byte {
	buf := new(bytes.Buffer)
	xml.Escape(buf, []byte(val))
	return buf.Bytes()
}

// NewPropNS returns a new PropertyXML instance
func NewPropNS(namespace string, local string, val string) *PropertyXML {
	return &PropertyXML{
		XMLName:  xml.Name{Space: namespace, Local: local},
		Lang:     "",
		InnerXML: xmlEscaped(val),
	}
}

// NewProp returns a new PropertyXML instance while xml-escaping the value
// TODO properly use the space
func NewProp(key, val string) *PropertyXML {
	return &PropertyXML{
		XMLName:  xml.Name{Space: "", Local: key},
		Lang:     "",
		InnerXML: xmlEscaped(val),
	}
}

// NewPropRaw returns a new PropertyXML instance for the given key/value pair
// TODO properly use the space
func NewPropRaw(key, val string) *PropertyXML {
	return &PropertyXML{
		XMLName:  xml.Name{Space: "", Local: key},
		Lang:     "",
		InnerXML: []byte(val),
	}
}

// Next returns the next token, if any, in the XML stream of d.
// RFC 4918 requires to ignore comments, processing instructions
// and directives.
// http://www.webdav.org/specs/rfc4918.html#property_values
// http://www.webdav.org/specs/rfc4918.html#xml-extensibility
func Next(d *xml.Decoder) (xml.Token, error) {
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

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_activelock
// <!ELEMENT activelock (lockscope, locktype, depth, owner?, timeout?,
//           locktoken?, lockroot)>
type ActiveLock struct {
	XMLName   xml.Name  `xml:"activelock"`
	Exclusive *struct{} `xml:"lockscope>exclusive,omitempty"`
	Shared    *struct{} `xml:"lockscope>shared,omitempty"`
	Write     *struct{} `xml:"locktype>write,omitempty"`
	Depth     string    `xml:"depth"`
	Owner     Owner     `xml:"owner,omitempty"`
	Timeout   string    `xml:"timeout,omitempty"`
	Locktoken string    `xml:"locktoken>href"`
	Lockroot  string    `xml:"lockroot>href,omitempty"`
}

// http://www.webdav.org/specs/rfc4918.html#ELEMENT_owner
type Owner struct {
	InnerXML string `xml:",innerxml"`
}

// FTXME remove once https://github.com/cs3org/cs3apis/pull/162 is merged
type LockDiscovery struct {
	// Opaque
	LockID     string
	Type       provider.LockType
	UserID     *userpb.UserId
	App        string
	Expiration *types.Timestamp
}

func Escape(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '&', '\'', '<', '>':
			b := bytes.NewBuffer(nil)
			xml.EscapeText(b, []byte(s))
			return b.String()
		}
	}
	return s
}
