package cback

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	v1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type cback struct {
	conf *Options
}

func init() {
	registry.Register("cback", New)
}

func New(m map[string]interface{}) (fs storage.FS, err error) {

	c := &Options{}
	if err = mapstructure.Decode(m, c); err != nil {
		return nil, errors.Wrap(err, "Error Decoding Configuration")
	}

	// returns the storage.FS interface
	return &cback{conf: c}, nil

}

func (fs *cback) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	var ssId, searchPath string

	user, _ := ctxpkg.ContextGetUser(ctx)
	UId, _ := ctxpkg.ContextGetUserID(ctx)

	resp, err := fs.matchBackups(user.Username, ref.Path)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	snapshotList, err := fs.listSnapshots(user.Username, resp.Id)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	for _, snapshot := range snapshotList {

		if snapshot.Id == resp.Substring {
			ssId = resp.Substring
			searchPath = resp.Source
			break

		} else if strings.HasPrefix(resp.Substring, snapshot.Id) {
			searchPath = strings.TrimPrefix(resp.Substring, snapshot.Id)
			searchPath = resp.Source + searchPath
			ssId = snapshot.Id
			break
		}
	}

	fmt.Printf("The ssId is: %v\nThe Path is %v\n", ssId, searchPath)
	ret, err := fs.statResource(resp.Id, ssId, user.Username, searchPath, resp.Source)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	time := v1beta1.Timestamp{
		Seconds: ret.Mtime,
		Nanos:   0,
	}

	ident := provider.ResourceId{
		OpaqueId:  ret.Path,
		StorageId: "cback",
	}

	checkSum := provider.ResourceChecksum{
		Sum:  "",
		Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
	}

	ri := &provider.ResourceInfo{
		Etag:          "",
		PermissionSet: &PermID,
		Checksum:      &checkSum,
		Mtime:         &time,
		Id:            &ident,
		Owner:         UId,
		Type:          provider.ResourceType(ret.Type),
		Size:          ret.Size,
		Path:          ret.Path,
	}

	if ret.Type == 2 {
		ri.MimeType = mime.Detect(true, ret.Path)
	} else {
		ri.MimeType = mime.Detect(false, ret.Path)
	}

	return ri, nil
}

func (fs *cback) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	//Implement this
	//Fix the ID part
	var path string = ref.GetPath()
	var ssId, searchPath string

	fmt.Print(path)

	user, _ := ctxpkg.ContextGetUser(ctx)
	UId, _ := ctxpkg.ContextGetUserID(ctx)

	resp, err := fs.matchBackups(user.Username, path)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	snapshotList, err := fs.listSnapshots(user.Username, resp.Id)

	if err != nil {
		fmt.Print(err)
		return nil, err
	}

	if resp.Substring != "" {
		for _, snapshot := range snapshotList {

			if snapshot.Id == resp.Substring {
				ssId = resp.Substring
				searchPath = resp.Source
				break

			} else if strings.HasPrefix(resp.Substring, snapshot.Id) {
				searchPath = strings.TrimPrefix(resp.Substring, snapshot.Id)
				searchPath = resp.Source + searchPath
				ssId = snapshot.Id
				break
			}
		}

		//If no match in path, therefore prints the files
		fmt.Printf("The ssId is: %v\nThe Path is %v\n", ssId, searchPath)
		ret, err := fs.fileSystem(resp.Id, ssId, user.Username, searchPath, resp.Source)

		if err != nil {
			fmt.Print(err)
			return nil, err
		}

		files := make([]*provider.ResourceInfo, len(ret))

		for index, j := range ret {

			time := v1beta1.Timestamp{
				Seconds: j.Mtime,
				Nanos:   0,
			}

			ident := provider.ResourceId{
				OpaqueId:  j.Path,
				StorageId: "cback",
			}

			checkSum := provider.ResourceChecksum{
				Sum:  "",
				Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
			}

			f := provider.ResourceInfo{
				Mtime:         &time,
				Id:            &ident,
				Checksum:      &checkSum,
				Path:          j.Path,
				Owner:         UId,
				PermissionSet: &PermID,
				Type:          provider.ResourceType(j.Type),
				Size:          j.Size,
				Etag:          "",
			}

			if j.Type == 2 {
				f.MimeType = mime.Detect(true, j.Path)
			} else {
				f.MimeType = mime.Detect(false, j.Path)
			}

			files[index] = &f
		}

		return files, nil

	} else {
		//If match in path, therefore prints the Snapshots
		files := make([]*provider.ResourceInfo, len(snapshotList))

		for index, snapshot := range snapshotList {

			epochTime, err := fs.timeConv(snapshot.Time)

			if err != nil {
				return nil, err
			}

			checkSum := provider.ResourceChecksum{
				Sum:  "",
				Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
			}

			ident := provider.ResourceId{
				OpaqueId:  ref.Path + "/" + snapshot.Id,
				StorageId: "cback",
			}

			time := v1beta1.Timestamp{
				Seconds: uint64(epochTime),
				Nanos:   0,
			}

			f := provider.ResourceInfo{

				Path:          ref.Path + "/" + snapshot.Id,
				Checksum:      &checkSum,
				Etag:          "",
				Owner:         UId,
				PermissionSet: &PermID,
				Id:            &ident,
				MimeType:      mime.Detect(true, ref.Path+"/"+snapshot.Id),
				Size:          0,

				//Check what time to put
				Mtime: &time,

				//Check Type
				Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			}
			files[index] = &f
			fmt.Printf("%v\n", snapshot.Id)
		}
		return files, nil
	}
}

func (fs *cback) Download(ctx context.Context, ref *provider.Reference) (rc io.ReadCloser, err error) {
	//Implement this
	return nil, errtypes.NotSupported("Operation Not Yet Implemented")
}

func (fs *cback) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) CreateHome(ctx context.Context) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) CreateDir(ctx context.Context, ref *provider.Reference) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) TouchFile(ctx context.Context, ref *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) GetPathByID(ctx context.Context, id *provider.ResourceId) (str string, err error) {
	return "", errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, used uint64, err error) {
	//Check if this is valid return
	return 0, 0, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) Shutdown(ctx context.Context) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (r *provider.CreateStorageSpaceResponse, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) ListRecycle(ctx context.Context, basePath, key, relativePath string) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (fs *cback) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	return errtypes.NotSupported("Operation Not Permitted")

}

func (fs *cback) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")

}
