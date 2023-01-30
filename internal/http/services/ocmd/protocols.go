package ocmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type Protocols []Protocol

type Protocol interface {
	ToOCMProtocol() *ocm.Protocol
}

// protocols supported by the OCM API

// WebDAV contains the parameters for the WebDAV protocol.
type WebDAV struct {
	SharedSecret string   `json:"sharedSecret" validate:"required"`
	Permissions  []string `json:"permissions" validate:"required,dive,required,oneof=read write share"`
	URL          string   `json:"url" validate:"required"`
}

func (w *WebDAV) ToOCMProtocol() *ocm.Protocol {
	perms := &ocm.SharePermissions{
		Permissions: &providerv1beta1.ResourcePermissions{},
	}
	for _, p := range w.Permissions {
		switch p {
		case "read":
			perms.Permissions.GetPath = true
			perms.Permissions.InitiateFileDownload = true
			perms.Permissions.ListContainer = true
			perms.Permissions.Stat = true
		case "write":
			perms.Permissions.InitiateFileUpload = true
		case "share":
			perms.Reshare = true
		}
	}

	return &ocm.Protocol{
		Term: &ocm.Protocol_WebdapOptions{
			WebdapOptions: &ocm.WebDAVProtocol{
				SharedSecret: w.SharedSecret,
				Uri:          w.URL,
				Permissions:  perms,
			},
		},
	}
}

// Webapp contains the parameters for the Webapp protocol.
type Webapp struct {
	URITemplate string `json:"uriTemplate" validate:"required"`
}

func (w *Webapp) ToOCMProtocol() *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_WebappOptions{
			WebappOptions: &ocm.WebappProtocol{
				UriTemplate: w.URITemplate,
			},
		},
	}
}

// Datatx contains the parameters for the Datatx protocol.
type Datatx struct {
	SharedSecret string `json:"sharedSecret" validate:"required"`
	SourceURI    string `json:"srcUri" validate:"required"`
	Size         uint64 `json:"size" validate:"required"`
}

func (w *Datatx) ToOCMProtocol() *ocm.Protocol {
	return &ocm.Protocol{
		Term: &ocm.Protocol_DatatxOprions{
			DatatxOprions: &ocm.DatatxProtocol{
				SharedSecret: w.SharedSecret,
				SourceUri:    w.SourceURI,
				Size:         w.Size,
			},
		},
	}
}

var protocolImpl = map[string]reflect.Type{
	"webdav": reflect.TypeOf(WebDAV{}),
	"webapp": reflect.TypeOf(Webapp{}),
	"datatx": reflect.TypeOf(Datatx{}),
}

func (p *Protocols) UnmarshalJSON(data []byte) error {
	var prot map[string]json.RawMessage
	if err := json.Unmarshal(data, &prot); err != nil {
		return err
	}

	*p = []Protocol{}

	for name, d := range prot {
		var res Protocol
		ctype, ok := protocolImpl[name]
		if !ok {
			return fmt.Errorf("protocol %s not recognised", name)
		}

		res = reflect.New(ctype).Interface().(Protocol)
		if err := json.Unmarshal(d, &res); err != nil {
			return err
		}

		*p = append(*p, res)
	}
	return nil
}

func (p Protocols) MarshalJSON() ([]byte, error) {
	d := make(map[string]Protocol)
	for _, prot := range p {
		d[getProtocolName(prot)] = prot
	}
	return json.Marshal(d)
}

func getProtocolName(p Protocol) string {
	n := reflect.TypeOf(p).String()
	s := strings.Split(n, ".")
	return strings.ToLower(s[len(s)-1])
}
