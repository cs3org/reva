// Copyright 2018-2024 CERN
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

//go:build ceph
// +build ceph

package cephfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	goceph "github.com/ceph/go-ceph/cephfs"
	"github.com/google/uuid"
)

// IsChunked checks if a given path refers to a chunk or not
func IsChunked(fn string) (bool, error) {
	// FIXME: also need to check whether the OC-Chunked header is set
	return regexp.MatchString(`-chunking-\w+-[0-9]+-[0-9]+$`, fn)
}

// ChunkBLOBInfo stores info about a particular chunk
// example: given /users/peter/myfile.txt-chunking-1234-10-2
type ChunkBLOBInfo struct {
	Path         string // example: /users/peter/myfile.txt
	TransferID   string // example: 1234
	TotalChunks  int    // example: 10
	CurrentChunk int    // example: 2
}

// Not using the resource path in the chunk folder name allows uploading to
// the same folder after a move without having to restart the chunk upload
func (c *ChunkBLOBInfo) uploadID() string {
	return fmt.Sprintf("chunking-%s-%d", c.TransferID, c.TotalChunks)
}

// GetChunkBLOBInfo decodes a chunk name to retrieve info about it.
func GetChunkBLOBInfo(path string) (*ChunkBLOBInfo, error) {
	parts := strings.Split(path, "-chunking-")
	tail := strings.Split(parts[1], "-")

	totalChunks, err := strconv.Atoi(tail[1])
	if err != nil {
		return nil, err
	}

	currentChunk, err := strconv.Atoi(tail[2])
	if err != nil {
		return nil, err
	}
	if currentChunk >= totalChunks {
		return nil, fmt.Errorf("current chunk:%d exceeds total number of chunks:%d", currentChunk, totalChunks)
	}

	return &ChunkBLOBInfo{
		Path:         parts[0],
		TransferID:   tail[0],
		TotalChunks:  totalChunks,
		CurrentChunk: currentChunk,
	}, nil
}

// ChunkHandler manages chunked uploads, storing the chunks in a temporary directory
// until it gets the final chunk which is then returned.
type ChunkHandler struct {
	user         *User
	uploadFolder string // example: /users/peter/.uploads
}

// NewChunkHandler creates a handler for chunked uploads.
func NewChunkHandler(ctx context.Context, fs *cephfs) *ChunkHandler {
	fmt.Println("debugging NewChunkHandler", fs.makeUser(ctx), fs.conf.UploadFolder)
	u := fs.makeUser(ctx)
	return &ChunkHandler{u, path.Join(u.home, fs.conf.UploadFolder)}
}

func (c *ChunkHandler) getTempFileName() string {
	return fmt.Sprintf("__%d_%s", time.Now().Unix(), uuid.New().String())
}

func (c *ChunkHandler) getAndCreateTransferFolderName(i *ChunkBLOBInfo) (path string, err error) {
	path = filepath.Join(c.uploadFolder, i.uploadID())
	c.user.op(func(cv *cacheVal) {
		err = cv.mount.MakeDir(path, 0777)
	})

	return
}

// TODO(labkode): I don't like how this function looks like, better to refactor
func (c *ChunkHandler) saveChunk(path string, r io.ReadCloser) (finish bool, chunk string, err error) {
	chunkInfo, err := GetChunkBLOBInfo(path)
	if err != nil {
		err = fmt.Errorf("error getting chunk info from path: %s", path)
		return
	}

	transferFolderName, err := c.getAndCreateTransferFolderName(chunkInfo)
	if err != nil {
		// TODO(labkode): skip error for now
		// err = fmt.Errorf("error getting transfer folder anme", err)
		return
	}
	fmt.Println("debugging: transferfoldername", transferFolderName)

	// here we write a temporary file that will be renamed to the transfer folder
	// with the correct sequence number filename.
	// we do not store this before-rename temporary files inside the transfer folder
	// to avoid errors when counting the number of chunks for finalizing the transfer.
	tmpFilename := c.getTempFileName()
	c.user.op(func(cv *cacheVal) {
		var tmpFile *goceph.File
		target := filepath.Join(c.uploadFolder, tmpFilename)
		fmt.Println("debugging savechunk, target: ", target)
		tmpFile, err = cv.mount.Open(target, os.O_CREATE|os.O_WRONLY, c.user.fs.conf.FilePerms)
		defer closeFile(tmpFile)
		if err != nil {
			return
		}
		_, err = io.Copy(tmpFile, r)
	})
	if err != nil {
		return
	}

	chunkTarget := filepath.Join(transferFolderName, strconv.Itoa(chunkInfo.CurrentChunk))
	c.user.op(func(cv *cacheVal) {
		err = cv.mount.Rename(tmpFilename, chunkTarget)
	})
	if err != nil {
		return
	}

	// Check that all chunks are uploaded.
	// This is very inefficient, the server has to check that it has all the
	// chunks after each uploaded chunk.
	// A two-phase upload like DropBox is better, because the server will
	// assembly the chunks when the client asks for it.
	numEntries := 0
	c.user.op(func(cv *cacheVal) {
		var dir *goceph.Directory
		var entry *goceph.DirEntry
		var chunkFile, assembledFile *goceph.File

		dir, err = cv.mount.OpenDir(transferFolderName)
		defer closeDir(dir)

		for entry, err = dir.ReadDir(); entry != nil && err == nil; entry, err = dir.ReadDir() {
			numEntries++
		}
		// to remove . and ..
		numEntries -= 2

		if err != nil || numEntries < chunkInfo.TotalChunks {
			return
		}

		// from now on we do have all the necessary chunks,
		// so we create a temporary file where all the chunks will be written
		// before being renamed to the requested location, from the example: /users/peter/myfile.txt

		assemblyFilename := filepath.Join(c.uploadFolder, c.getTempFileName())
		assembledFile, err = cv.mount.Open(assemblyFilename, os.O_CREATE|os.O_WRONLY, c.user.fs.conf.FilePerms)
		defer closeFile(assembledFile)
		defer deleteFile(cv.mount, assemblyFilename)
		if err != nil {
			return
		}

		for i := 0; i < numEntries; i++ {
			target := filepath.Join(transferFolderName, strconv.Itoa(i))

			chunkFile, err = cv.mount.Open(target, os.O_RDONLY, 0)
			if err != nil {
				return
			}
			_, err = io.Copy(assembledFile, chunkFile)
			closeFile(chunkFile)
			if err != nil {
				return
			}
		}

		// clean all the chunks that made the assembly file
		for i := 0; i < numEntries; i++ {
			target := filepath.Join(transferFolderName, strconv.Itoa(i))
			err = cv.mount.Unlink(target)
			if err != nil {
				return
			}
		}
	})
	return
}

// WriteChunk saves an intermediate chunk temporarily and assembles all chunks
// once the final one is received.
// this function will return the original filename (myfile.txt) and the assemblyPath when
// the upload is completed
func (c *ChunkHandler) WriteChunk(fn string, r io.ReadCloser) (string, string, error) {
	finish, chunk, err := c.saveChunk(fn, r)
	if err != nil {
		return "", "", err
	}

	if !finish {
		return "", "", nil
	}

	chunkInfo, err := GetChunkBLOBInfo(fn)
	if err != nil {
		return "", "", err
	}

	return chunkInfo.Path, chunk, nil
}
