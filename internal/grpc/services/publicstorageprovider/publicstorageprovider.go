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

package publicstorageprovider

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

func init() {
	rgrpc.Register("publicstorageprovider", New)
}

type config struct {
	MountPath        string                            `mapstructure:"mount_path"`
	MountID          string                            `mapstructure:"mount_id"`
	GatewayAddr      string                            `mapstructure:"gateway_addr"`
	Driver           string                            `mapstructure:"driver"`
	Drivers          map[string]map[string]interface{} `mapstructure:"drivers"`
	DriverAddr       string                            `mapstructure:"driver_addr"`
	DataServerURL    string                            `mapstructure:"data_server_url"`
	DataServerPrefix string                            `mapstructure:"data_server_prefix"`
}

type service struct {
	conf               *config
	mountPath, mountID string
	gateway            gateway.GatewayAPIClient
	storage            storage.FS
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new Public Storage Provider service.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	mountPath := c.MountPath
	mountID := c.MountID

	fs, err := getFS(c)
	if err != nil {
		return nil, err
	}

	gateway, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:      c,
		mountPath: mountPath,
		mountID:   mountID,
		gateway:   gateway,
		storage:   fs,
	}

	return service, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	statReq := &provider.StatRequest{Ref: req.Ref}
	statRes, err := s.Stat(ctx, statReq)
	if err != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &provider.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found"),
			}, nil
		}
		err := status.NewErrorFromCode(statRes.Status.Code, "gateway")
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref"),
		}, nil
	}

	req.Opaque = statRes.Info.Opaque
	return s.initiateFileDownload(ctx, req)
}

// PathTranslator encapsulates behavior that requires translation between external paths into internal paths. Internal
// path depend on the storage provider in use, hence a translation is needed. In the case of Public Links, the translation
// goes beyond a simple path conversion but instead, the token needs to be "expanded" to an internal path. Essentially
// transforming:
// "YzUTlrKrpswo/foldera/folderb/file.txt"
// into:
// "shared-folder-path/internal-folder/foldera/folderb/file.txt".
type PathTranslator struct {
	dir   string
	base  string
	token string
}

// NewPathTranslator creates a new PathTranslator.
func NewPathTranslator(p string) *PathTranslator {
	return &PathTranslator{
		dir:   filepath.Dir(p),
		base:  filepath.Base(p),
		token: strings.Split(p, "/")[2],
	}
}

// Both, t.dir and tokenPath paths need to be merged:
// tokenPath   = /oc/einstein/public-links
// t.dir       = /public/ausGxuUePCOi/foldera/folderb/
// res         = /public-links/foldera/folderb/
// this `res` will get then expanded taking into account the authenticated user and the storage:
// end         = /einstein/files/public-links/foldera/folderb/

// TODO how would this behave with a storage other than owncloud?
// the current OC namespace looks like "/oc/einstein/public-links"
// and operations on it would be hardcoded string operations bound to
// the storage path. Either we need clearly defined paths from each storage,
// or else this would remain black magic.
func (s *service) initiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	t := NewPathTranslator(req.Ref.GetPath())
	tokenPath, err := s.pathFromToken(ctx, t.token)
	if err != nil {
		return nil, err
	}

	var destURL *url.URL

	// handle the case where a single file is shared publicly, for such case the url.Path might only be: "/public/ItYJjJNdiuEn"
	// TODO the "/public" prefix might be configurable, although a prefix, nested paths should not be allowed, as in: `/public/level1/level2` values as prefix.
	if len(strings.Split(req.Ref.GetPath(), "/")) == 3 {
		base := strings.Join(strings.Split(tokenPath, "/")[3:], "/")
		destURL, err = url.Parse(strings.Join([]string{s.conf.DataServerURL, s.conf.DataServerPrefix, base}, "/"))
		if err != nil {
			return nil, err
		}
	} else if isOCStorage(tokenPath) {
		base := strings.Join(strings.Split(tokenPath, "/")[3:], "/")
		request := strings.Join(strings.Split(t.dir, "/")[3:], "/")
		t.dir = strings.Join([]string{base, request}, "/")

		destURL, err = url.Parse(strings.Join([]string{s.conf.DataServerURL, s.conf.DataServerPrefix, t.dir, t.base}, "/"))
		if err != nil {
			return nil, err
		}
	}

	return &provider.InitiateFileDownloadResponse{
		Opaque:           req.Opaque,
		Status:           &rpc.Status{Code: rpc.Code_CODE_OK},
		DownloadEndpoint: destURL.String(),
		Expose:           true,
	}, nil
}

func isOCStorage(q string) bool {
	return strings.HasPrefix(q, "/oc/")
}

func (s *service) dataURL() (*url.URL, error) {
	target := strings.Join([]string{s.conf.DataServerURL, s.conf.DataServerPrefix}, "/")
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	return targetURL, nil
}

func (s *service) uploadFullPath(ctx context.Context, refPath string) (string, error) {
	var fullPath []string
	// req.Ref.GetPath() = "/public/{token}/subfolderA/subfolderB/file.txt"
	token := strings.Split(refPath, "/")[2]

	ref, err := s.refFromToken(ctx, token)
	if err != nil {
		return "", err
	}

	sharedRootPath := strings.Split(ref.GetPath(), "/")[3:] // internal paths have the storage prefixed, i.e: "/oc/einstein/asdf/""
	fullPath = append(sharedRootPath, strings.Split(refPath, "/")[3:]...)
	// i.e: fullPath = /sharedFolder/subfolder/file.txt
	return strings.Join(fullPath, "/"), nil
}

func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	logger := appctx.GetLogger(ctx)

	fullPath, err := s.uploadFullPath(ctx, req.Ref.GetPath())
	if err != nil {
		return nil, err
	}

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: fullPath,
		},
	}

	targetURL, err := s.dataURL()
	if err != nil {
		return nil, err
	}

	var uploadLength int64
	if req.Opaque != nil && req.Opaque.Map != nil && req.Opaque.Map["Upload-Length"] != nil {
		var err error
		uploadLength, err = strconv.ParseInt(string(req.Opaque.Map["Upload-Length"].Value), 10, 64)
		if err != nil {
			return &provider.InitiateFileUploadResponse{
				Status: status.NewInternal(ctx, err, "error parsing upload length"),
			}, nil
		}
	}

	uploadID, err := s.storage.InitiateUpload(ctx, ref, uploadLength, nil)
	if err != nil {
		return &provider.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error getting upload id"),
		}, nil
	}
	targetURL.Path = path.Join("/", targetURL.Path, uploadID)

	logger.Info().Str("data-server", targetURL.String()).
		Str("fn", req.Ref.GetPath()).
		Str("xs", fmt.Sprintf("%+v", "false")).
		Msg("file upload")
	res := &provider.InitiateFileUploadResponse{
		UploadEndpoint: targetURL.String(),
		Status:         status.NewOK(ctx),
		Expose:         true,
		// AvailableChecksums: ,
	}

	return res, nil
}

func (s *service) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	ctx, span := trace.StartSpan(ctx, "Stat")
	defer span.End()

	span.AddAttributes(
		trace.StringAttribute("ref", req.Ref.String()),
	)

	tkn, relativePath, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	pathFromToken, err := s.pathFromToken(ctx, tkn)
	if err != nil {
		return nil, err
	}

	// the call has to be made to the gateway instead of the storage.
	statResponse, err := s.gateway.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path.Join("/", pathFromToken, relativePath),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if statResponse.Info != nil {
		statResponse.Info.Path = path.Join("/", tkn, relativePath)
	}

	return statResponse, nil
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	tkn, relativePath, err := s.unwrap(ctx, req.Ref)
	if err != nil {
		return nil, err
	}

	pathFromToken, err := s.pathFromToken(ctx, tkn)
	if err != nil {
		return nil, err
	}

	listContainerR, err := s.gateway.ListContainer(
		ctx,
		&provider.ListContainerRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: path.Join("/", pathFromToken, relativePath),
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	for i := range listContainerR.Infos {
		listContainerR.Infos[i].Path = path.Join("/", tkn, relativePath, path.Base(listContainerR.Infos[i].Path))
	}

	return listContainerR, nil
}

func (s *service) unwrap(ctx context.Context, ref *provider.Reference) (token string, relativePath string, err error) {
	if ref.GetId() != nil {
		return "", "", errors.New("need path based ref: got " + ref.String())
	}

	if ref.GetPath() == "" {
		// abort, no valid id nor path
		return "", "", errors.New("invalid ref: " + ref.String())
	}

	// i.e path: /public/{token}/path/to/subfolders
	fn := ref.GetPath()
	// fsfn: /{token}/path/to/subfolders
	fsfn, err := s.trimMountPrefix(fn)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(fsfn, "/", 3)
	token = parts[1]
	if len(parts) > 2 {
		relativePath = parts[2]
	}

	return
}

func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycleStream(req *provider.ListRecycleStreamRequest, ss provider.ProviderAPI_ListRecycleStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) AddGrant(ctx context.Context, req *provider.AddGrantRequest) (*provider.AddGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateReference(ctx context.Context, req *provider.CreateReferenceRequest) (*provider.CreateReferenceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateGrant(ctx context.Context, req *provider.UpdateGrantRequest) (*provider.UpdateGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RemoveGrant(ctx context.Context, req *provider.RemoveGrantRequest) (*provider.RemoveGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) trimMountPrefix(fn string) (string, error) {
	if strings.HasPrefix(fn, s.mountPath) {
		return path.Join("/", strings.TrimPrefix(fn, s.mountPath)), nil
	}
	return "", errors.New(fmt.Sprintf("path=%q does not belong to this storage provider mount path=%q"+fn, s.mountPath))
}

// pathFromToken returns the path for the publicly shared resource.
func (s *service) pathFromToken(ctx context.Context, token string) (string, error) {
	driver, err := pool.GetPublicShareProviderClient(s.conf.DriverAddr)
	if err != nil {
		return "", err
	}

	publicShareResponse, err := driver.GetPublicShare(
		ctx,
		&link.GetPublicShareRequest{
			Ref: &link.PublicShareReference{
				Spec: &link.PublicShareReference_Token{
					Token: token,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	pathRes, err := s.gateway.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: publicShareResponse.GetShare().GetResourceId(),
	})
	if err != nil {
		return "", err
	}

	return pathRes.Path, nil
}

// refFromToken returns the path for the publicly shared resource.
func (s *service) refFromToken(ctx context.Context, token string) (*provider.Reference, error) {
	driver, err := pool.GetPublicShareProviderClient(s.conf.DriverAddr)
	if err != nil {
		return nil, err
	}

	publicShareResponse, err := driver.GetPublicShare(
		ctx,
		&link.GetPublicShareRequest{
			Ref: &link.PublicShareReference{
				Spec: &link.PublicShareReference_Token{
					Token: token,
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	pathRes, err := s.gateway.GetPath(ctx, &provider.GetPathRequest{
		ResourceId: publicShareResponse.GetShare().GetResourceId(),
	})
	if err != nil {
		return nil, err
	}

	return &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: pathRes.Path,
		},
	}, nil
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}
