//Package store provides a storage backend based on the local file system.
//
// OwnCloudStore is a storage backend used as a handler.DataStore in handler.NewHandler.
// It stores the uploads in a directory specified in two different files: The
// `[id].info` files are used to store the fileinfo in JSON format. The
// `[id]` files without an extension contain the raw binary data uploaded.
// No cleanup is performed so you may want to run a cronjob to ensure your disk
// is not filled up with old and finished uploads.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/tus/tusd/internal/uid"
	"github.com/tus/tusd/pkg/handler"
)

var defaultFilePerm = os.FileMode(0664)

// OwnCloudStore handles storage inside a local posix filesystem thet follows the legacy owncloud datadir layout.
// See the handler.DataStore interface for documentation about the different methods.
// TODO decide how we identify files
// - by path?
// - by id? we may not have a file id? but we may have a parent id
// - expect as metadata? does that violate the standard?
// TODO difference between upload and download ids
// - uploads are temporary ids ... file id and name are given in the metadata
// - downloads? cannot use the upload id ... tus does not deal with GET. so we could use the fileid in the url to identify which file to download
// - datasvc uses the path ... because eos primarily works on paths
// - IMO we should work on ids ... or better use the path or id as preferred by the CS3 storage provider. the initiate download url will return a custom id ...
//   we could make that create the id that is used to download the file via tus.
// TODO should the storageprovider create the id / send the POST request to the datasvc? This would follow the spec.
// TODO Do we even need the initiate up / download in the storageprovider? I would like to save a few requests.
// - leaving them in the storage provider allows it to participate in up/downloads ...
// - the initiate upload could add workflow steps to the POST request, eg to set up antivirus scan or filtering? or just a callback to trigger the propagation?
// - propagation should be handled by the configuration ... keep in mind that the datasvc is intended to be open to the clients.
//   well we could disable the creation extension and let that be handled by the storag provider .... hm nice
// TODO The name storageprovider no longer seems to capture what it does. All it does is handle metadata.
// TODO if the tus/data service is responsible for workflows, how is the file made available in the actual storage implementation?
// - for the owncloud driver the file needs to get a new fileid, or, if it overwrites an existing file, it needs to retain its fileid AND metadata
//   - if we use fileids in the first place this is fine, because we already have either the file id or the parent id
//   - 1. the old data needs to be stored as a version: read fileid?, move to files_versions
//   - 2. the new data needs to be put in place: write fileid, move to the target dir
//   - 3. start propagation: responsibility of the owncloud driver?
// - for the eos driver the file upload can happen directly to the file (if using REST PUT & ranges) because it will only overwrite the file if it is completely written
//   - 1. the old data needs to be stored as a version: done by eos internally
//   - 2. the new data needs to be put in place: done by eos internally
//   - 3. start propagation: done by eos internally
// TODO refactor the metadata propagator as a standalone service. It can be omitted for eos, but eg owncloud and s3 need it
// TODO move the Upload code from the storage driver to tus, return not implemmented in the storage drive?
// TODO rename tusdsvc to datasvc?

type OwnCloudStore struct {
	// Relative or absolute path to the data dir. OwnCloudStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
}

// New creates a new ownCloud data dir based storage backend. The directory specified will
// be used as the only storage entry. This method does not check
// whether the path exists, use os.MkdirAll to ensure.
// In addition, a locking mechanism is provided.
func New(path string) OwnCloudStore {
	return OwnCloudStore{path}
}

// UseIn sets this store as the core data store in the passed composer and adds
// all possible extension to it.
func (store OwnCloudStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
}

// NewUpload is called by the storage provider?
func (store OwnCloudStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	id := uid.Uid()
	binPath := store.binPath(id)
	info.ID = id
	info.Storage = map[string]string{
		"Type": "OwnCloudStore",
		"Path": binPath,
	}

	// Create binary file with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("upload directory does not exist: %s", store.Path)
		}
		return nil, err
	}
	defer file.Close()

	upload := &fileUpload{
		info:     info,
		infoPath: store.infoPath(id),
		binPath:  store.binPath(id),
	}

	// writeInfo creates the file by itself if necessary
	err = upload.writeInfo()
	if err != nil {
		return nil, err
	}

	return upload, nil
}

func (store OwnCloudStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	info := handler.FileInfo{}
	data, err := ioutil.ReadFile(store.infoPath(id))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	binPath := store.binPath(id)
	infoPath := store.infoPath(id)
	stat, err := os.Stat(binPath)
	if err != nil {
		return nil, err
	}

	info.Offset = stat.Size()

	return &fileUpload{
		info:     info,
		binPath:  binPath,
		infoPath: infoPath,
	}, nil
}

func (store OwnCloudStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store OwnCloudStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store OwnCloudStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

// binPath returns the path to the file storing the binary data.
func (store OwnCloudStore) binPath(id string) string {
	return filepath.Join(store.Path, id)
}

// infoPath returns the path to the .info file storing the file's info.
func (store OwnCloudStore) infoPath(id string) string {
	return filepath.Join(store.Path, id+".info")
}

type fileUpload struct {
	// info stores the current information about the upload
	info handler.FileInfo
	// infoPath is the path to the .info file
	infoPath string
	// binPath is the path to the binary file (which has no extension)
	binPath string
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	return upload.info, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	// If the HTTP PATCH request gets interrupted in the middle (e.g. because
	// the user wants to pause the upload), Go's net/http returns an io.ErrUnexpectedEOF.
	// However, for OwnCloudStore it's not important whether the stream has ended
	// on purpose or accidentally.
	if err == io.ErrUnexpectedEOF {
		err = nil
	}

	upload.info.Offset += n

	return n, err
}

func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	if err := os.Remove(upload.infoPath); err != nil {
		return err
	}
	if err := os.Remove(upload.binPath); err != nil {
		return err
	}
	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(fileUpload.binPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(upload.infoPath, data, defaultFilePerm)
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	return nil
}
