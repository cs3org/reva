package walk

import (
	"context"
	"path/filepath"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

type WalkFunc func(path string, info *provider.ResourceInfo, err error) error

func Walk(ctx context.Context, root string, gtw gateway.GatewayAPIClient, fn WalkFunc) error {
	info, err := stat(ctx, root, gtw)

	if err != nil {
		return fn(root, nil, err)
	}

	err = walkRecursively(ctx, root, info, gtw, fn)

	if err == filepath.SkipDir {
		return nil
	}

	return err
}

func walkRecursively(ctx context.Context, path string, info *provider.ResourceInfo, gtw gateway.GatewayAPIClient, fn WalkFunc) error {

	if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return fn(path, info, nil)
	}

	list, err := readDir(ctx, path, gtw)
	errFn := fn(path, info, err)

	if err != nil || errFn != nil {
		return errFn
	}

	for _, file := range list {
		err = walkRecursively(ctx, file.Path, file, gtw, fn)
		if err != nil && (file.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER || err != filepath.SkipDir) {
			return err
		}
	}

	return nil
}

func readDir(ctx context.Context, path string, gtw gateway.GatewayAPIClient) ([]*provider.ResourceInfo, error) {
	resp, err := gtw.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(resp.Status.Message)
	}

	return resp.Infos, nil
}

func stat(ctx context.Context, path string, gtw gateway.GatewayAPIClient) (*provider.ResourceInfo, error) {
	resp, err := gtw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: path,
		},
	})

	switch {
	case err != nil:
		return nil, err
	case resp.Status.Code != rpc.Code_CODE_OK:
		return nil, errtypes.InternalError(resp.Status.Message)
	}

	return resp.Info, nil
}
