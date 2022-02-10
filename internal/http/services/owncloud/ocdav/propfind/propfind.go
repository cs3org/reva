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

package propfind

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/props"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

//go:generate mockery -name GatewayClient

type countingReader struct {
	n int
	r io.Reader
}

// Props represents properties related to a resource
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_prop (for propfind)
type Props []xml.Name

// XML holds the xml representation of a propfind
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propfind
type XML struct {
	XMLName  xml.Name  `xml:"DAV: propfind"`
	Allprop  *struct{} `xml:"DAV: allprop"`
	Propname *struct{} `xml:"DAV: propname"`
	Prop     Props     `xml:"DAV: prop"`
	Include  Props     `xml:"DAV: include"`
}

// PropstatXML holds the xml representation of a propfind response
// http://www.webdav.org/specs/rfc4918.html#ELEMENT_propstat
type PropstatXML struct {
	// Prop requires DAV: to be the default namespace in the enclosing
	// XML. This is due to the standard encoding/xml package currently
	// not honoring namespace declarations inside a xmltag with a
	// parent element for anonymous slice elements.
	// Use of multistatusWriter takes care of this.
	Prop                []*props.PropertyXML `xml:"d:prop>_ignored_"`
	Status              string               `xml:"d:status"`
	Error               *errors.ErrorXML     `xml:"d:error"`
	ResponseDescription string               `xml:"d:responsedescription,omitempty"`
}

// ResponseXML holds the xml representation of a propfind response
type ResponseXML struct {
	XMLName             xml.Name         `xml:"d:response"`
	Href                string           `xml:"d:href"`
	Propstat            []PropstatXML    `xml:"d:propstat"`
	Status              string           `xml:"d:status,omitempty"`
	Error               *errors.ErrorXML `xml:"d:error"`
	ResponseDescription string           `xml:"d:responsedescription,omitempty"`
}

// MultiStatusResponseXML holds the xml representation of a multistatus propfind response
type MultiStatusResponseXML struct {
	XMLName xml.Name `xml:"d:multistatus"`
	XmlnsS  string   `xml:"xmlns:s,attr,omitempty"`
	XmlnsD  string   `xml:"xmlns:d,attr,omitempty"`
	XmlnsOC string   `xml:"xmlns:oc,attr,omitempty"`

	Responses []*ResponseXML `xml:"d:response"`
}

// ResponseUnmarshalXML is a workaround for https://github.com/golang/go/issues/13400
type ResponseUnmarshalXML struct {
	XMLName             xml.Name               `xml:"response"`
	Href                string                 `xml:"href"`
	Propstat            []PropstatUnmarshalXML `xml:"propstat"`
	Status              string                 `xml:"status,omitempty"`
	Error               *errors.ErrorXML       `xml:"d:error"`
	ResponseDescription string                 `xml:"responsedescription,omitempty"`
}

// MultiStatusResponseUnmarshalXML is a workaround for https://github.com/golang/go/issues/13400
type MultiStatusResponseUnmarshalXML struct {
	XMLName xml.Name `xml:"multistatus"`
	XmlnsS  string   `xml:"xmlns:s,attr,omitempty"`
	XmlnsD  string   `xml:"xmlns:d,attr,omitempty"`
	XmlnsOC string   `xml:"xmlns:oc,attr,omitempty"`

	Responses []*ResponseUnmarshalXML `xml:"response"`
}

// PropstatUnmarshalXML is a workaround for https://github.com/golang/go/issues/13400
type PropstatUnmarshalXML struct {
	// Prop requires DAV: to be the default namespace in the enclosing
	// XML. This is due to the standard encoding/xml package currently
	// not honoring namespace declarations inside a xmltag with a
	// parent element for anonymous slice elements.
	// Use of multistatusWriter takes care of this.
	Prop                []*props.PropertyXML `xml:"prop"`
	Status              string               `xml:"status"`
	Error               *errors.ErrorXML     `xml:"d:error"`
	ResponseDescription string               `xml:"responsedescription,omitempty"`
}

// NewMultiStatusResponseXML returns a preconfigured instance of MultiStatusResponseXML
func NewMultiStatusResponseXML() *MultiStatusResponseXML {
	return &MultiStatusResponseXML{
		XmlnsD:  "DAV:",
		XmlnsS:  "http://sabredav.org/ns",
		XmlnsOC: "http://owncloud.org/ns",
	}
}

// GatewayClient is the interface that's being used to interact with the gateway
type GatewayClient interface {
	gateway.GatewayAPIClient
}

// GetGatewayServiceClientFunc is a callback used to pass in a StorageProviderClient during testing
type GetGatewayServiceClientFunc func() (GatewayClient, error)

// Handler handles propfind requests
type Handler struct {
	PublicURL string
	getClient GetGatewayServiceClientFunc
}

// NewHandler returns a new PropfindHandler instance
func NewHandler(publicURL string, getClientFunc GetGatewayServiceClientFunc) *Handler {
	return &Handler{
		PublicURL: publicURL,
		getClient: getClientFunc,
	}
}

// HandlePathPropfind handles a path based propfind request
// ns is the namespace that is prefixed to the path in the cs3 namespace
func (p *Handler) HandlePathPropfind(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), fmt.Sprintf("%s %v", r.Method, r.URL.Path))
	defer span.End()

	span.SetAttributes(attribute.String("component", "ocdav"))

	fn := path.Join(ns, r.URL.Path) // TODO do we still need to jail if we query the registry about the spaces?

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	pf, status, err := ReadPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// retrieve a specific storage space
	client, err := p.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error retrieving a gateway service client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	spaces, rpcStatus, err := spacelookup.LookUpStorageSpacesForPathWithChildren(ctx, client, fn)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	resourceInfos, sendTusHeaders, ok := p.getResourceInfos(ctx, w, r, pf, spaces, fn, false, sublog)
	if !ok {
		// getResourceInfos handles responses in case of an error so we can just return here.
		return
	}
	p.propfindResponse(ctx, w, r, ns, pf, sendTusHeaders, resourceInfos, sublog)
}

// HandleSpacesPropfind handles a spaces based propfind request
func (p *Handler) HandleSpacesPropfind(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "spaces_propfind")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Logger()
	client, err := p.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pf, status, err := ReadPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// retrieve a specific storage space
	space, rpcStatus, err := spacelookup.LookUpStorageSpaceByID(ctx, client, spaceID)
	if err != nil {
		sublog.Error().Err(err).Msg("error looking up the space by id")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	resourceInfos, sendTusHeaders, ok := p.getResourceInfos(ctx, w, r, pf, []*provider.StorageSpace{space}, r.URL.Path, true, sublog)
	if !ok {
		// getResourceInfos handles responses in case of an error so we can just return here.
		return
	}

	// prefix space id to paths
	for i := range resourceInfos {
		resourceInfos[i].Path = path.Join("/", spaceID, resourceInfos[i].Path)
	}

	p.propfindResponse(ctx, w, r, "", pf, sendTusHeaders, resourceInfos, sublog)

}

func (p *Handler) propfindResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, namespace string, pf XML, sendTusHeaders bool, resourceInfos []*provider.ResourceInfo, log zerolog.Logger) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(ctx, "propfind_response")
	defer span.End()

	filters := make([]*link.ListPublicSharesRequest_Filter, 0, len(resourceInfos))
	for i := range resourceInfos {
		filters = append(filters, publicshare.ResourceIDFilter(resourceInfos[i].Id))
	}

	client, err := p.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var linkshares map[string]struct{}
	listResp, err := client.ListPublicShares(ctx, &link.ListPublicSharesRequest{Filters: filters})
	if err == nil {
		linkshares = make(map[string]struct{}, len(listResp.Share))
		for i := range listResp.Share {
			linkshares[listResp.Share[i].ResourceId.OpaqueId] = struct{}{}
		}
	} else {
		log.Error().Err(err).Msg("propfindResponse: couldn't list public shares")
		span.SetStatus(codes.Error, err.Error())
	}

	propRes, err := MultistatusResponse(ctx, &pf, resourceInfos, p.PublicURL, namespace, linkshares)
	if err != nil {
		log.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(net.HeaderDav, "1, 3, extended-mkcol")
	w.Header().Set(net.HeaderContentType, "application/xml; charset=utf-8")

	if sendTusHeaders {
		w.Header().Add(net.HeaderAccessControlExposeHeaders, strings.Join([]string{net.HeaderTusResumable, net.HeaderTusVersion, net.HeaderTusExtension}, ", "))
		w.Header().Set(net.HeaderTusResumable, "1.0.0")
		w.Header().Set(net.HeaderTusVersion, "1.0.0")
		w.Header().Set(net.HeaderTusExtension, "creation,creation-with-upload,checksum,expiration")
	}

	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write(propRes); err != nil {
		log.Err(err).Msg("error writing response")
	}
}

// TODO this is just a stat -> rename
func (p *Handler) statSpace(ctx context.Context, client gateway.GatewayAPIClient, space *provider.StorageSpace, ref *provider.Reference, metadataKeys []string) (*provider.ResourceInfo, *rpc.Status, error) {
	req := &provider.StatRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: metadataKeys,
	}
	res, err := client.Stat(ctx, req)
	if err != nil || res == nil || res.Status == nil || res.Status.Code != rpc.Code_CODE_OK {
		return nil, res.Status, err
	}
	return res.Info, res.Status, nil
}

func (p *Handler) getResourceInfos(ctx context.Context, w http.ResponseWriter, r *http.Request, pf XML, spaces []*provider.StorageSpace, requestPath string, spacesPropfind bool, log zerolog.Logger) ([]*provider.ResourceInfo, bool, bool) {
	dh := r.Header.Get(net.HeaderDepth)
	depth, err := net.ParseDepth(dh)
	if err != nil {
		log.Debug().Str("depth", dh).Msg(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		m := fmt.Sprintf("Invalid Depth header value: %v", dh)
		b, err := errors.Marshal(http.StatusBadRequest, m, "")
		errors.HandleWebdavError(&log, w, b, err)
		return nil, false, false
	}

	client, err := p.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false, false
	}

	var metadataKeys []string

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

	// we need to stat all spaces to aggregate the root etag, mtime and size
	// TODO cache per space (hah, no longer per user + per space!)
	var rootInfo *provider.ResourceInfo
	var mostRecentChildInfo *provider.ResourceInfo
	var aggregatedChildSize uint64
	spaceInfos := make([]*provider.ResourceInfo, 0, len(spaces))
	spaceMap := map[*provider.ResourceInfo]*provider.Reference{}
	for _, space := range spaces {
		if space.Opaque == nil || space.Opaque.Map == nil || space.Opaque.Map["path"] == nil || space.Opaque.Map["path"].Decoder != "plain" {
			continue // not mounted
		}
		spacePath := string(space.Opaque.Map["path"].Value)
		// TODO separate stats to the path or to the children, after statting all children update the mtime/etag
		// TODO get mtime, and size from space as well, so we no longer have to stat here?
		spaceRef := spacelookup.MakeRelativeReference(space, requestPath, spacesPropfind)
		info, status, err := p.statSpace(ctx, client, space, spaceRef, metadataKeys)
		if err != nil || status.Code != rpc.Code_CODE_OK {
			continue
		}

		// adjust path
		if spacesPropfind {
			// we need to prefix the path with / to make subsequent prefix matches work
			info.Path = filepath.Join("/", spaceRef.Path)
		} else {
			info.Path = filepath.Join(spacePath, spaceRef.Path)
		}

		spaceMap[info] = spaceRef
		spaceInfos = append(spaceInfos, info)
		if rootInfo == nil && (requestPath == info.Path || (spacesPropfind && requestPath == path.Join("/", info.Path))) {
			rootInfo = info
		} else if requestPath != spacePath && strings.HasPrefix(spacePath, requestPath) { // Check if the space is a child of the requested path
			// aggregate child metadata
			aggregatedChildSize += info.Size
			if mostRecentChildInfo == nil {
				mostRecentChildInfo = info
				continue
			}
			if mostRecentChildInfo.Mtime == nil || (info.Mtime != nil && utils.TSToUnixNano(info.Mtime) > utils.TSToUnixNano(mostRecentChildInfo.Mtime)) {
				mostRecentChildInfo = info
			}
		}
	}

	if len(spaceInfos) == 0 || rootInfo == nil {
		// TODO if we have children invent node on the fly
		w.WriteHeader(http.StatusNotFound)
		m := fmt.Sprintf("Resource %v not found", requestPath)
		b, err := errors.Marshal(http.StatusNotFound, m, "")
		errors.HandleWebdavError(&log, w, b, err)
		return nil, false, false
	}
	if mostRecentChildInfo != nil {
		if rootInfo.Mtime == nil || (mostRecentChildInfo.Mtime != nil && utils.TSToUnixNano(mostRecentChildInfo.Mtime) > utils.TSToUnixNano(rootInfo.Mtime)) {
			rootInfo.Mtime = mostRecentChildInfo.Mtime
			if mostRecentChildInfo.Etag != "" {
				rootInfo.Etag = mostRecentChildInfo.Etag
			}
		}
		if rootInfo.Etag == "" {
			rootInfo.Etag = mostRecentChildInfo.Etag
		}
	}

	// add size of children
	rootInfo.Size += aggregatedChildSize

	resourceInfos := []*provider.ResourceInfo{
		rootInfo, // PROPFIND always includes the root resource
	}
	if rootInfo.Type == provider.ResourceType_RESOURCE_TYPE_FILE {
		// no need to stat any other spaces, we got our file stat already
		return resourceInfos, true, true
	}

	childInfos := map[string]*provider.ResourceInfo{}
	addChild := func(spaceInfo *provider.ResourceInfo) {
		if spaceInfo == rootInfo {
			return // already accounted for
		}

		childPath := strings.TrimPrefix(spaceInfo.Path, requestPath)
		childName, tail := router.ShiftPath(childPath)
		if tail != "/" {
			spaceInfo.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
			spaceInfo.Checksum = nil
			// TODO unset opaque checksum
		}
		spaceInfo.Path = path.Join(requestPath, childName)
		if existingChild, ok := childInfos[childName]; ok {
			// use most recent child
			if existingChild.Mtime == nil || (spaceInfo.Mtime != nil && utils.TSToUnixNano(spaceInfo.Mtime) > utils.TSToUnixNano(existingChild.Mtime)) {
				childInfos[childName].Mtime = spaceInfo.Mtime
				childInfos[childName].Etag = spaceInfo.Etag
				childInfos[childName].Size = childInfos[childName].Size + spaceInfo.Size
			}
			// only update fileid if the resource is a direct child
			if tail == "/" {
				childInfos[childName].Id = spaceInfo.Id
			}
		} else {
			childInfos[childName] = spaceInfo
		}
	}
	// then add children
	for _, spaceInfo := range spaceInfos {
		switch {
		case !spacesPropfind && spaceInfo.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER && depth != net.DepthInfinity:
			addChild(spaceInfo)

		case spaceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER && depth == net.DepthOne:
			switch {
			case strings.HasPrefix(requestPath, spaceInfo.Path):
				req := &provider.ListContainerRequest{
					Ref:                   spaceMap[spaceInfo],
					ArbitraryMetadataKeys: metadataKeys,
				}
				res, err := client.ListContainer(ctx, req)
				if err != nil {
					log.Error().Err(err).Msg("error sending list container grpc request")
					w.WriteHeader(http.StatusInternalServerError)
					return nil, false, false
				}

				if res.Status.Code != rpc.Code_CODE_OK {
					log.Debug().Interface("status", res.Status).Msg("List Container not ok, skipping")
					continue
				}
				for _, info := range res.Infos {
					info.Path = path.Join(requestPath, info.Path)
				}
				resourceInfos = append(resourceInfos, res.Infos...)
			case strings.HasPrefix(spaceInfo.Path, requestPath): // space is a deep child of the requested path
				addChild(spaceInfo)
			default:
				log.Debug().Msg("unhandled")
			}

		case depth == net.DepthInfinity:
			// use a stack to explore sub-containers breadth-first
			if spaceInfo != rootInfo {
				resourceInfos = append(resourceInfos, spaceInfo)
			}
			stack := []*provider.ResourceInfo{spaceInfo}
			for len(stack) != 0 {
				info := stack[0]
				stack = stack[1:]

				if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					continue
				}
				req := &provider.ListContainerRequest{
					Ref: &provider.Reference{
						ResourceId: spaceInfo.Id,
						// TODO here we cut of the path that we added after stating the space above
						Path: utils.MakeRelativePath(strings.TrimPrefix(info.Path, spaceInfo.Path)),
					},
					ArbitraryMetadataKeys: metadataKeys,
				}
				res, err := client.ListContainer(ctx, req) // FIXME public link depth infinity -> "gateway: could not find provider: gateway: error calling ListStorageProviders: rpc error: code = PermissionDenied desc = auth: core access token is invalid"
				if err != nil {
					log.Error().Err(err).Interface("info", info).Msg("error sending list container grpc request")
					w.WriteHeader(http.StatusInternalServerError)
					return nil, false, false
				}
				if res.Status.Code != rpc.Code_CODE_OK {
					log.Debug().Interface("status", res.Status).Msg("List Container not ok, skipping")
					continue
				}

				// check sub-containers in reverse order and add them to the stack
				// the reversed order here will produce a more logical sorting of results
				for i := len(res.Infos) - 1; i >= 0; i-- {
					// add path to resource
					res.Infos[i].Path = filepath.Join(info.Path, res.Infos[i].Path)
					if spacesPropfind {
						res.Infos[i].Path = utils.MakeRelativePath(res.Infos[i].Path)
					}
					if res.Infos[i].Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						stack = append(stack, res.Infos[i])
					}
				}

				resourceInfos = append(resourceInfos, res.Infos...)
				// TODO: stream response to avoid storing too many results in memory
				// we can do that after having stated the root.
			}
		}
	}

	if rootInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// now add all aggregated child infos
		for _, childInfo := range childInfos {
			resourceInfos = append(resourceInfos, childInfo)
		}
	}

	sendTusHeaders := true
	// let clients know this collection supports tus.io POST requests to start uploads
	if rootInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if rootInfo.Opaque != nil {
			_, ok := rootInfo.Opaque.Map["disable_tus"]
			sendTusHeaders = !ok
		}
	}

	return resourceInfos, sendTusHeaders, true
}

func requiresExplicitFetching(n *xml.Name) bool {
	switch n.Space {
	case net.NsDav:
		switch n.Local {
		case "quota-available-bytes", "quota-used-bytes":
			//  A <DAV:allprop> PROPFIND request SHOULD NOT return DAV:quota-available-bytes and DAV:quota-used-bytes
			// from https://www.rfc-editor.org/rfc/rfc4331.html#section-2
			return true
		default:
			return false
		}
	case net.NsOwncloud:
		switch n.Local {
		case "favorite", "share-types", "checksums", "size":
			return true
		default:
			return false
		}
	case net.NsOCS:
		return false
	}
	return true
}

// ReadPropfind extracts and parses the propfind XML information from a Reader
// from https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/xml.go#L178-L205
func ReadPropfind(r io.Reader) (pf XML, status int, err error) {
	c := countingReader{r: r}
	if err = xml.NewDecoder(&c).Decode(&pf); err != nil {
		if err == io.EOF {
			if c.n == 0 {
				// An empty body means to propfind allprop.
				// http://www.webdav.org/specs/rfc4918.html#METHOD_PROPFIND
				return XML{Allprop: new(struct{})}, 0, nil
			}
			err = errors.ErrInvalidPropfind
		}
		return XML{}, http.StatusBadRequest, err
	}

	if pf.Allprop == nil && pf.Include != nil {
		return XML{}, http.StatusBadRequest, errors.ErrInvalidPropfind
	}
	if pf.Allprop != nil && (pf.Prop != nil || pf.Propname != nil) {
		return XML{}, http.StatusBadRequest, errors.ErrInvalidPropfind
	}
	if pf.Prop != nil && pf.Propname != nil {
		return XML{}, http.StatusBadRequest, errors.ErrInvalidPropfind
	}
	if pf.Propname == nil && pf.Allprop == nil && pf.Prop == nil {
		// jfd: I think <d:prop></d:prop> is perfectly valid ... treat it as allprop
		return XML{Allprop: new(struct{})}, 0, nil
	}
	return pf, 0, nil
}

// MultistatusResponse converts a list of resource infos into a multistatus response string
func MultistatusResponse(ctx context.Context, pf *XML, mds []*provider.ResourceInfo, publicURL, ns string, linkshares map[string]struct{}) ([]byte, error) {
	responses := make([]*ResponseXML, 0, len(mds))
	for i := range mds {
		res, err := mdToPropResponse(ctx, pf, mds[i], publicURL, ns, linkshares)
		if err != nil {
			return nil, err
		}
		responses = append(responses, res)
	}

	msr := NewMultiStatusResponseXML()
	msr.Responses = responses
	msg, err := xml.Marshal(msr)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// mdToPropResponse converts the CS3 metadata into a webdav PropResponse
// ns is the CS3 namespace that needs to be removed from the CS3 path before
// prefixing it with the baseURI
func mdToPropResponse(ctx context.Context, pf *XML, md *provider.ResourceInfo, publicURL, ns string, linkshares map[string]struct{}) (*ResponseXML, error) {
	sublog := appctx.GetLogger(ctx).With().Interface("md", md).Str("ns", ns).Logger()
	md.Path = strings.TrimPrefix(md.Path, ns)

	baseURI := ctx.Value(net.CtxKeyBaseURI).(string)

	ref := path.Join(baseURI, md.Path)
	if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		ref += "/"
	}

	response := ResponseXML{
		Href:     net.EncodePath(ref),
		Propstat: []PropstatXML{},
	}

	var ls *link.PublicShare

	// -1 indicates uncalculated
	// -2 indicates unknown (default)
	// -3 indicates unlimited
	quota := net.PropQuotaUnknown
	size := strconv.FormatUint(md.Size, 10)
	var lock *provider.Lock
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
		if md.Opaque.Map["lock"] != nil && md.Opaque.Map["lock"].Decoder == "json" {
			lock = &provider.Lock{}
			err := json.Unmarshal(md.Opaque.Map["lock"].Value, lock)
			if err != nil {
				sublog.Error().Err(err).Msg("could not unmarshal locks json")
			}
		}
	}

	role := conversions.RoleFromResourcePermissions(md.PermissionSet)

	isShared := !net.IsCurrentUserOwner(ctx, md.Owner)
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

	propstatOK := PropstatXML{
		Status: "HTTP/1.1 200 OK",
		Prop:   []*props.PropertyXML{},
	}
	propstatNotFound := PropstatXML{
		Status: "HTTP/1.1 404 Not Found",
		Prop:   []*props.PropertyXML{},
	}
	// when allprops has been requested
	if pf.Allprop != nil {
		// return all known properties

		if md.Id != nil {
			id := resourceid.OwnCloudResourceIDWrap(md.Id)
			propstatOK.Prop = append(propstatOK.Prop,
				props.NewProp("oc:id", id),
				props.NewProp("oc:fileid", id),
			)
		}

		if md.Etag != "" {
			// etags must be enclosed in double quotes and cannot contain them.
			// See https://tools.ietf.org/html/rfc7232#section-2.3 for details
			// TODO(jfd) handle weak tags that start with 'W/'
			propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getetag", quoteEtag(md.Etag)))
		}

		if md.PermissionSet != nil {
			propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:permissions", wdp))
		}

		// always return size, well nearly always ... public link shares are a little weird
		if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("d:resourcetype", "<d:collection/>"))
			if ls == nil {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:size", size))
			}
			// A <DAV:allprop> PROPFIND request SHOULD NOT return DAV:quota-available-bytes and DAV:quota-used-bytes
			// from https://www.rfc-editor.org/rfc/rfc4331.html#section-2
			// propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:quota-used-bytes", size))
			// propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:quota-available-bytes", quota))
		} else {
			propstatOK.Prop = append(propstatOK.Prop,
				props.NewProp("d:resourcetype", ""),
				props.NewProp("d:getcontentlength", size),
			)
			if md.MimeType != "" {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontenttype", md.MimeType))
			}
		}
		// Finder needs the getLastModified property to work.
		if md.Mtime != nil {
			t := utils.TSToTime(md.Mtime).UTC()
			lastModifiedString := t.Format(net.RFC1123)
			propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getlastmodified", lastModifiedString))
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
				checksums.Write(e.Value)
			}
			if e, ok := md.Opaque.Map["adler32"]; ok {
				if checksums.Len() == 0 {
					checksums.WriteString("<oc:checksum>ADLER32:")
				} else {
					checksums.WriteString(" ADLER32:")
				}
				checksums.Write(e.Value)
			}
		}
		if checksums.Len() > 0 {
			checksums.WriteString("</oc:checksum>")
			propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("oc:checksums", checksums.String()))
		}

		// ls do not report any properties as missing by default
		if ls == nil {
			// favorites from arbitrary metadata
			if k := md.GetArbitraryMetadata(); k == nil {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
			} else if amd := k.GetMetadata(); amd == nil {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
			} else if v, ok := amd[net.PropOcFavorite]; ok && v != "" {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", v))
			} else {
				propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
			}
		}

		if lock != nil {
			propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("d:lockdiscovery", activeLocks(&sublog, lock)))
		}
		// TODO return other properties ... but how do we put them in a namespace?
	} else {
		// otherwise return only the requested properties
		for i := range pf.Prop {
			switch pf.Prop[i].Space {
			case net.NsOwncloud:
				switch pf.Prop[i].Local {
				// TODO(jfd): maybe phoenix and the other clients can just use this id as an opaque string?
				// I tested the desktop client and phoenix to annotate which properties are requestted, see below cases
				case "fileid": // phoenix only
					if md.Id != nil {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:fileid", resourceid.OwnCloudResourceIDWrap(md.Id)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:fileid", ""))
					}
				case "id": // desktop client only
					if md.Id != nil {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:id", resourceid.OwnCloudResourceIDWrap(md.Id)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:id", ""))
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
					propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:permissions", wdp))
				case "public-link-permission": // only on a share root node
					if ls != nil && md.PermissionSet != nil {
						propstatOK.Prop = append(
							propstatOK.Prop,
							props.NewProp("oc:public-link-permission", strconv.FormatUint(uint64(role.OCSPermissions()), 10)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-permission", ""))
					}
				case "public-link-item-type": // only on a share root node
					if ls != nil {
						if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:public-link-item-type", "folder"))
						} else {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:public-link-item-type", "file"))
							// redirectref is another option
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-item-type", ""))
					}
				case "public-link-share-datetime":
					if ls != nil && ls.Mtime != nil {
						t := utils.TSToTime(ls.Mtime).UTC() // TODO or ctime?
						shareTimeString := t.Format(net.RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:public-link-share-datetime", shareTimeString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-share-datetime", ""))
					}
				case "public-link-share-owner":
					if ls != nil && ls.Owner != nil {
						if net.IsCurrentUserOwner(ctx, ls.Owner) {
							u := ctxpkg.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:public-link-share-owner", u.Username))
						} else {
							u, _ := ctxpkg.ContextGetUser(ctx)
							sublog.Error().Interface("share", ls).Interface("user", u).Msg("the current user in the context should be the owner of a public link share")
							propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-share-owner", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-share-owner", ""))
					}
				case "public-link-expiration":
					if ls != nil && ls.Expiration != nil {
						t := utils.TSToTime(ls.Expiration).UTC()
						expireTimeString := t.Format(net.RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:public-link-expiration", expireTimeString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-expiration", ""))
					}
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:public-link-expiration", ""))
				case "size": // phoenix only
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					// TODO what is the difference to d:quota-used-bytes (which only exists for collections)?
					// oc:size is available on files and folders and behaves like d:getcontentlength or d:quota-used-bytes respectively
					// The hasPrefix is a workaround to make children of the link root show a size if they have 0 bytes
					if ls == nil || strings.HasPrefix(md.Path, "/"+ls.Token+"/") {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:size", size))
					} else {
						// link share root collection has no size
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:size", ""))
					}
				case "owner-id": // phoenix only
					if md.Owner != nil {
						if net.IsCurrentUserOwner(ctx, md.Owner) {
							u := ctxpkg.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:owner-id", u.Username))
						} else {
							sublog.Debug().Msg("TODO fetch user username")
							propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:owner-id", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:owner-id", ""))
					}
				case "favorite": // phoenix only
					// TODO: can be 0 or 1?, in oc10 it is present or not
					// TODO: read favorite via separate call? that would be expensive? I hope it is in the md
					// TODO: this boolean favorite property is so horribly wrong ... either it is presont, or it is not ... unless ... it is possible to have a non binary value ... we need to double check
					if ls == nil {
						if k := md.GetArbitraryMetadata(); k == nil {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
						} else if amd := k.GetMetadata(); amd == nil {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
						} else if v, ok := amd[net.PropOcFavorite]; ok && v != "" {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "1"))
						} else {
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:favorite", "0"))
						}
					} else {
						// link share root collection has no favorite
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:favorite", ""))
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
							checksums.Write(e.Value)
						}
						if e, ok := md.Opaque.Map["adler32"]; ok {
							if checksums.Len() == 0 {
								checksums.WriteString("<oc:checksum>ADLER32:")
							} else {
								checksums.WriteString(" ADLER32:")
							}
							checksums.Write(e.Value)
						}
					}
					if checksums.Len() > 13 {
						checksums.WriteString("</oc:checksum>")
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("oc:checksums", checksums.String()))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:checksums", ""))
					}
				case "share-types": // desktop
					var types strings.Builder
					k := md.GetArbitraryMetadata()
					amd := k.GetMetadata()
					if amdv, ok := amd[metadataKeyOf(&pf.Prop[i])]; ok {
						types.WriteString("<oc:share-type>")
						types.WriteString(amdv)
						types.WriteString("</oc:share-type>")
					}

					if md.Id != nil {
						if _, ok := linkshares[md.Id.OpaqueId]; ok {
							types.WriteString("<oc:share-type>3</oc:share-type>")
						}
					}

					if types.Len() != 0 {
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("oc:share-types", types.String()))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:"+pf.Prop[i].Local, ""))
					}
				case "owner-display-name": // phoenix only
					if md.Owner != nil {
						if net.IsCurrentUserOwner(ctx, md.Owner) {
							u := ctxpkg.ContextMustGetUser(ctx)
							propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:owner-display-name", u.DisplayName))
						} else {
							sublog.Debug().Msg("TODO fetch user displayname")
							propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:owner-display-name", ""))
						}
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:owner-display-name", ""))
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
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("oc:downloadURL", publicURL+baseURI+path))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:"+pf.Prop[i].Local, ""))
					}
				case "signature-auth":
					if isPublic {
						// We only want to add the attribute to the root of the propfind.
						if strings.HasSuffix(md.Path, ls.Token) && ls.Signature != nil {
							expiration := time.Unix(int64(ls.Signature.SignatureExpiration.Seconds), int64(ls.Signature.SignatureExpiration.Nanos))
							var sb strings.Builder
							sb.WriteString("<oc:signature>")
							sb.WriteString(ls.Signature.Signature)
							sb.WriteString("</oc:signature>")
							sb.WriteString("<oc:expiration>")
							sb.WriteString(expiration.Format(time.RFC3339))
							sb.WriteString("</oc:expiration>")

							propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("oc:signature-auth", sb.String()))
						} else {
							propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:signature-auth", ""))
						}
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
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("oc:"+pf.Prop[i].Local, ""))
				}
			case net.NsDav:
				switch pf.Prop[i].Local {
				case "getetag": // both
					if md.Etag != "" {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getetag", quoteEtag(md.Etag)))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getetag", ""))
					}
				case "getcontentlength": // both
					// see everts stance on this https://stackoverflow.com/a/31621912, he points to http://tools.ietf.org/html/rfc4918#section-15.3
					// > Purpose: Contains the Content-Length header returned by a GET without accept headers.
					// which only would make sense when eg. rendering a plain HTML filelisting when GETing a collection,
					// which is not the case ... so we don't return it on collections. owncloud has oc:size for that
					// TODO we cannot find out if md.Size is set or not because ints in go default to 0
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getcontentlength", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontentlength", size))
					}
				case "resourcetype": // both
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("d:resourcetype", "<d:collection/>"))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:resourcetype", ""))
						// redirectref is another option
					}
				case "getcontenttype": // phoenix
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// directories have no contenttype
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getcontenttype", ""))
					} else if md.MimeType != "" {
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getcontenttype", md.MimeType))
					}
				case "getlastmodified": // both
					// TODO we cannot find out if md.Mtime is set or not because ints in go default to 0
					if md.Mtime != nil {
						t := utils.TSToTime(md.Mtime).UTC()
						lastModifiedString := t.Format(net.RFC1123)
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:getlastmodified", lastModifiedString))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:getlastmodified", ""))
					}
				case "quota-used-bytes": // RFC 4331
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// always returns the current usage,
						// in oc10 there seems to be a bug that makes the size in webdav differ from the one in the user properties, not taking shares into account
						// in ocis we plan to always mak the quota a property of the storage space
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:quota-used-bytes", size))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:quota-used-bytes", ""))
					}
				case "quota-available-bytes": // RFC 4331
					if md.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
						// oc10 returns -3 for unlimited, -2 for unknown, -1 for uncalculated
						propstatOK.Prop = append(propstatOK.Prop, props.NewProp("d:quota-available-bytes", quota))
					} else {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:quota-available-bytes", ""))
					}
				case "lockdiscovery": // http://www.webdav.org/specs/rfc2518.html#PROPERTY_lockdiscovery
					if lock == nil {
						propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:lockdiscovery", ""))
					} else {
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropRaw("d:lockdiscovery", activeLocks(&sublog, lock)))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:"+pf.Prop[i].Local, ""))
				}
			case net.NsOCS:
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
						propstatOK.Prop = append(propstatOK.Prop, props.NewPropNS(pf.Prop[i].Space, pf.Prop[i].Local, strconv.FormatUint(uint64(perms), 10)))
					}
				default:
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewProp("d:"+pf.Prop[i].Local, ""))
				}
			default:
				// handle custom properties
				if k := md.GetArbitraryMetadata(); k == nil {
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				} else if amd := k.GetMetadata(); amd == nil {
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
				} else if v, ok := amd[metadataKeyOf(&pf.Prop[i])]; ok && v != "" {
					propstatOK.Prop = append(propstatOK.Prop, props.NewPropNS(pf.Prop[i].Space, pf.Prop[i].Local, v))
				} else {
					propstatNotFound.Prop = append(propstatNotFound.Prop, props.NewPropNS(pf.Prop[i].Space, pf.Prop[i].Local, ""))
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

func activeLocks(log *zerolog.Logger, lock *provider.Lock) string {
	if lock == nil || lock.Type == provider.LockType_LOCK_TYPE_INVALID {
		return ""
	}
	expiration := "Infinity"
	if lock.Expiration != nil {
		now := uint64(time.Now().Unix())
		// Should we hide expired locks here? No.
		//
		// If the timeout expires, then the lock SHOULD be removed.  In this
		// case the server SHOULD act as if an UNLOCK method was executed by the
		// server on the resource using the lock token of the timed-out lock,
		// performed with its override authority.
		//
		// see https://datatracker.ietf.org/doc/html/rfc4918#section-6.6
		if lock.Expiration.Seconds >= now {
			expiration = "Second-" + strconv.FormatUint(lock.Expiration.Seconds-now, 10)
		} else {
			expiration = "Second-0"
		}
	}

	// xml.Encode cannot render emptytags like <d:write/>, see https://github.com/golang/go/issues/21399
	var activelocks strings.Builder
	activelocks.WriteString("<d:activelock>")
	// webdav locktype write | transaction
	switch lock.Type {
	case provider.LockType_LOCK_TYPE_EXCL:
		fallthrough
	case provider.LockType_LOCK_TYPE_WRITE:
		activelocks.WriteString("<d:locktype><d:write/></d:locktype>")
	}
	// webdav lockscope exclusive, shared, or local
	switch lock.Type {
	case provider.LockType_LOCK_TYPE_EXCL:
		fallthrough
	case provider.LockType_LOCK_TYPE_WRITE:
		activelocks.WriteString("<d:lockscope><d:exclusive/></d:lockscope>")
	case provider.LockType_LOCK_TYPE_SHARED:
		activelocks.WriteString("<d:lockscope><d:shared/></d:lockscope>")
	}
	// we currently only support depth infinity
	activelocks.WriteString("<d:depth>Infinity</d:depth>")

	if lock.User != nil || lock.AppName != "" {
		activelocks.WriteString("<d:owner>")

		if lock.User != nil {
			// TODO oc10 uses displayname and email, needs a user lookup
			activelocks.WriteString(props.Escape(lock.User.OpaqueId + "@" + lock.User.Idp))
		}
		if lock.AppName != "" {
			if lock.User != nil {
				activelocks.WriteString(" via ")
			}
			activelocks.WriteString(props.Escape(lock.AppName))
		}
		activelocks.WriteString("</d:owner>")
	}
	activelocks.WriteString("<d:timeout>")
	activelocks.WriteString(expiration)
	activelocks.WriteString("</d:timeout>")
	if lock.LockId != "" {
		activelocks.WriteString("<d:locktoken><d:href>")
		activelocks.WriteString(props.Escape(lock.LockId))
		activelocks.WriteString("</d:href></d:locktoken>")
	}
	// lockroot is only used when setting the lock
	activelocks.WriteString("</d:activelock>")
	return activelocks.String()
}

// be defensive about wrong encoded etags
func quoteEtag(etag string) string {
	if strings.HasPrefix(etag, "W/") {
		return `W/"` + strings.Trim(etag[2:], `"`) + `"`
	}
	return `"` + strings.Trim(etag, `"`) + `"`
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func metadataKeyOf(n *xml.Name) string {
	switch {
	case n.Space == net.NsDav && n.Local == "quota-available-bytes":
		return "quota"
	default:
		return fmt.Sprintf("%s/%s", n.Space, n.Local)
	}
}

// UnmarshalXML appends the property names enclosed within start to pn.
//
// It returns an error if start does not contain any properties or if
// properties contain values. Character data between properties is ignored.
func (pn *Props) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		t, err := props.Next(d)
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
			t, err = props.Next(d)
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
