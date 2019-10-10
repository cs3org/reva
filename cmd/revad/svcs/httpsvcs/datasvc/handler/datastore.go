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

// this file is a fork of https://github.com/tus/tusd/tree/master/pkg/handler/datastore.go
// TODO remove when PRs have been merged upstream

package handler

import (
	"context"
	"io"
)

// MetaData is a simple key value map for arbitrary metadata
type MetaData map[string]string

// FileInfo holds the current offset of an upload as well as other metadata
type FileInfo struct {
	ID string
	// Total file size in bytes specified in the NewUpload call
	Size int64
	// Indicates whether the total file size is deferred until later
	SizeIsDeferred bool
	// Offset in bytes (zero-based)
	Offset   int64
	MetaData MetaData
	// Indicates that this is a partial upload which will later be used to form
	// a final upload by concatenation. Partial uploads should not be processed
	// when they are finished since they are only incomplete chunks of files.
	IsPartial bool
	// Indicates that this is a final upload
	IsFinal bool
	// If the upload is a final one (see IsFinal) this will be a non-empty
	// ordered slice containing the ids of the uploads of which the final upload
	// will consist after concatenation.
	PartialUploads []string
	// Storage contains information about where the data storage saves the upload,
	// for example a file path. The available values vary depending on what data
	// store is used. This map may also be nil.
	Storage map[string]string

	// stopUpload is the cancel function for the upload's context.Context. When
	// invoked it will interrupt the writes to DataStore#WriteChunk.
	stopUpload context.CancelFunc
}

// StopUpload interrupts an running upload from the server-side. This means that
// the current request body is closed, so that the data store does not get any
// more data. Furthermore, a response is sent to notify the client of the
// interrupting and the upload is terminated (if supported by the data store),
// so the upload cannot be resumed anymore.
func (f FileInfo) StopUpload() {
	if f.stopUpload != nil {
		f.stopUpload()
	}
}

// Upload is the interface that needs to be implemented by uploads for the
// core protocol
type Upload interface {
	// Write the chunk read from src into the file specified by the id at the
	// given offset. The handler will take care of validating the offset and
	// limiting the size of the src to not overflow the file's size. It may
	// return an os.ErrNotExist which will be interpreted as a 404 Not Found.
	// It will also lock resources while they are written to ensure only one
	// write happens per time.
	// The function call must return the number of bytes written.
	WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error)
	// Read the fileinformation used to validate the offset and respond to HEAD
	// requests. It may return an os.ErrNotExist which will be interpreted as a
	// 404 Not Found.
	GetInfo(ctx context.Context) (FileInfo, error)
	// GetReader returns a reader which allows iterating of the content of an
	// upload specified by its ID. It should attempt to provide a reader even if
	// the upload has not been finished yet but it's not required.
	// If the returned reader also implements the io.Closer interface, the
	// Close() method will be invoked once everything has been read.
	// If the given upload could not be found, the error tusd.ErrNotFound should
	// be returned.
	GetReader(ctx context.Context) (io.Reader, error)
	// FinisherDataStore is the interface which can be implemented by DataStores
	// which need to do additional operations once an entire upload has been
	// completed. These tasks may include but are not limited to freeing unused
	// resources or notifying other services. For example, S3Store uses this
	// interface for removing a temporary object.
	// FinishUpload executes additional operations for the finished upload which
	// is specified by its ID.
	FinishUpload(ctx context.Context) error
}

// DataStore is the interface that needs to be implemented for the core protocol
type DataStore interface {
	GetUpload(ctx context.Context, id string) (upload Upload, err error)
}

// CreatingDataStore is the interface which must be implemented by DataStores
// if they want to receive POST requests using the Handler. If this interface
// is not implemented, no request handler for this method is attached.
type CreatingDataStore interface {
	// Create a new upload using the size as the file's length. The method must
	// return an unique id which is used to identify the upload. If no backend
	// (e.g. Riak) specifes the id you may want to use the uid package to
	// generate one. The properties Size and MetaData will be filled.
	NewUpload(ctx context.Context, info FileInfo) (upload Upload, err error)
}

// TerminatableUpload is the interface that needs to be implemented by an upload
// for the termination extension
type TerminatableUpload interface {
	// Terminate an upload so any further requests to the resource, both reading
	// and writing, must return os.ErrNotExist or similar.
	Terminate(ctx context.Context) error
}

// TerminaterDataStore is the interface which must be implemented by DataStores
// if they want to receive DELETE requests using the Handler. If this interface
// is not implemented, no request handler for this method is attached.
type TerminaterDataStore interface {
	AsTerminatableUpload(upload Upload) TerminatableUpload
}

// ConcaterDataStore is the interface required to be implemented if the
// Concatenation extension should be enabled. Only in this case, the handler
// will parse and respect the Upload-Concat header.
type ConcaterDataStore interface {
	AsConcatableUpload(upload Upload) ConcatableUpload
}

// ConcatableUpload is the interface that needs to be implemented by an upload
// for the concatenation extension
type ConcatableUpload interface {
	// ConcatUploads concatenates the content from the provided partial uploads
	// and writes the result in the destination upload.
	// The caller (usually the handler) must and will ensure that this
	// destination upload has been created before with enough space to hold all
	// partial uploads. The order, in which the partial uploads are supplied,
	// must be respected during concatenation.
	ConcatUploads(ctx context.Context, partialUploads []Upload) error
}

// LengthDeferrerDataStore is the interface that must be implemented if the
// creation-defer-length extension should be enabled. The extension enables a
// client to upload files when their total size is not yet known. Instead, the
// client must send the total size as soon as it becomes known.
type LengthDeferrerDataStore interface {
	AsLengthDeclarableUpload(upload Upload) LengthDeclarableUpload
}

// LengthDeclarableUpload is the interface Uploads need to implement for the
// creation-defer-length extension
type LengthDeclarableUpload interface {
	DeclareLength(ctx context.Context, length int64) error
}

// Locker is the interface required for custom lock persisting mechanisms.
// Common ways to store this information is in memory, on disk or using an
// external service, such as Redis.
// When multiple processes are attempting to access an upload, whether it be
// by reading or writing, a synchronization mechanism is required to prevent
// data corruption, especially to ensure correct offset values and the proper
// order of chunks inside a single upload.
type Locker interface {
	// NewLock creates a new unlocked lock object for the given upload ID.
	NewLock(id string) (Lock, error)
}

// Lock is the interface for a lock as returned from a Locker.
type Lock interface {
	// Lock attempts to obtain an exclusive lock for the upload specified
	// by its id.
	// If this operation fails because the resource is already locked, the
	// tusd.ErrFileLocked must be returned. If no error is returned, the attempt
	// is consider to be successful and the upload to be locked until UnlockUpload
	// is invoked for the same upload.
	Lock() error
	// Unlock releases an existing lock for the given upload.
	Unlock() error
}
