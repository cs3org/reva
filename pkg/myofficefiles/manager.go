package myofficefiles

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
)

type OfficeFileType string

const (
	TypeWord       OfficeFileType = "doc"
	TypeExcel      OfficeFileType = "xls"
	TypePowerpoint OfficeFileType = "ppt"
)

// Manager defines an interface for a MyOfficeFiles manager.
type Manager interface {
	// ListMyOfficeFiles returns all recent Office files of a user.
	ListMyOfficeFiles(ctx context.Context, user *userpb.User, filetype OfficeFileType) ([]*provider.ResourceInfo, error)
}

// This feature is only enabled for users that are in the targetGroup
const (
	targetGroup = "cernbox-office-view"
	depth       = 10
)

var officeFilesRegex = map[OfficeFileType]string{
	TypeWord:       "(.*?)(.doc|.DOC)[x|X]?$",
	TypeExcel:      "(.*?)(.xls|.XLS|)[x|X]?$",
	TypePowerpoint: "(.*?)(.ppt|.PPT)[x|X]?$",
}

type svc struct {
	gateway gatewayv1beta1.GatewayAPIClient
}

func New(ctx context.Context, gatewayEndpoint string) (Manager, error) {
	gateway, err := pool.GetGatewayServiceClient(pool.Endpoint(gatewayEndpoint))
	if err != nil {
		return nil, err
	}

	return &svc{
		gateway: gateway,
	}, nil
}

func FileType(filetype string) (OfficeFileType, error) {
	switch filetype {
	case "doc":
		return TypeWord, nil
	case "xls":
		return TypeExcel, nil
	case "ppt":
		return TypePowerpoint, nil
	default:
		return "", errors.New("Invalid filetype")
	}
}

func (s *svc) ListMyOfficeFiles(ctx context.Context, user *userpb.User, filetype OfficeFileType) ([]*provider.ResourceInfo, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("ListMyOfficeFiles")

	if !slices.Contains(user.Groups, targetGroup) {
		return nil, errtypes.PermissionDenied("ListMyOfficeFiles is only enabled for users in the " + targetGroup + " group")
	}

	u := appctx.ContextMustGetUser(ctx)
	home := templates.WithUser(u, "/eos/user/{{substr 0 1 .Username}}/{{.Username}}/")

	// home and all projects
	// TODO: get user projects
	paths := []string{
		home,
	}
	var regex = officeFilesRegex[filetype]

	resourceInfos := []*provider.ResourceInfo{}

	for _, path := range paths {
		log.Info().Str("path", path).Msg("ListMyOfficeFiles")
		res, err := s.gateway.ListContainer(ctx, &provider.ListContainerRequest{
			Opaque: &typesv1beta1.Opaque{
				Map: map[string]*typesv1beta1.OpaqueEntry{
					"regex": {
						Decoder: "plain",
						Value:   []byte(regex),
					},
					"depth": {
						Decoder: "plain",
						Value:   []byte(strconv.Itoa(depth)),
					},
				},
			},
			Ref: &provider.Reference{
				Path: path,
			},
		})

		if err != nil {
			return nil, err
		}

		if res.Status == nil || res.Status.Code != rpc.Code_CODE_OK {
			return nil, fmt.Errorf("error during ListContainer: %s", res.Status.String())
		}

		resourceInfos = append(resourceInfos, res.Infos...)
	}

	return resourceInfos, nil
}
