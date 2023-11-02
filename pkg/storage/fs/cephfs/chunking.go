// Copyright 2018-2023 CERN
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
type ChunkBLOBInfo struct {
	Path         string
	TransferID   string
	TotalChunks  int
	CurrentChunk int
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
	user        *User
	chunkFolder string
}

// NewChunkHandler creates a handler for chunked uploads.
func NewChunkHandler(ctx context.Context, fs *cephfs) *ChunkHandler {
	return &ChunkHandler{fs.makeUser(ctx), fs.conf.UploadFolder}
}

func (c *ChunkHandler) getChunkTempFileName() string {
	return fmt.Sprintf("__%d_%s", time.Now().Unix(), uuid.New().String())
}

func (c *ChunkHandler) getChunkFolderName(i *ChunkBLOBInfo) (path string, err error) {
	path = filepath.Join(c.chunkFolder, i.uploadID())
	c.user.op(func(cv *cacheVal) {
		err = cv.mount.MakeDir(path, 0777)
	})

	return
}

func (c *ChunkHandler) saveChunk(path string, r io.ReadCloser) (finish bool, chunk string, err error) {
	chunkInfo, err := GetChunkBLOBInfo(path)
	if err != nil {
		err = fmt.Errorf("error getting chunk info from path: %s", path)
		return
	}

	chunkTempFilename := c.getChunkTempFileName()
	c.user.op(func(cv *cacheVal) {
		var tmpFile *goceph.File
		target := filepath.Join(c.chunkFolder, chunkTempFilename)
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

	chunksFolderName, err := c.getChunkFolderName(chunkInfo)
	if err != nil {
		return
	}
	// c.logger.Info().Log("chunkfolder", chunksFolderName)

	chunkTarget := filepath.Join(chunksFolderName, strconv.Itoa(chunkInfo.CurrentChunk))
	c.user.op(func(cv *cacheVal) {
		err = cv.mount.Rename(chunkTempFilename, chunkTarget)
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

		dir, err = cv.mount.OpenDir(chunksFolderName)
		defer closeDir(dir)

		for entry, err = dir.ReadDir(); entry != nil && err == nil; entry, err = dir.ReadDir() {
			numEntries++
		}
		// to remove . and ..
		numEntries -= 2

		if err != nil || numEntries < chunkInfo.TotalChunks {
			return
		}

		chunk = filepath.Join(c.chunkFolder, c.getChunkTempFileName())
		assembledFile, err = cv.mount.Open(chunk, os.O_CREATE|os.O_WRONLY, c.user.fs.conf.FilePerms)
		defer closeFile(assembledFile)
		defer deleteFile(cv.mount, chunk)
		if err != nil {
			return
		}

		for i := 0; i < numEntries; i++ {
			target := filepath.Join(chunksFolderName, strconv.Itoa(i))

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

		// necessary approach in case assembly fails
		for i := 0; i < numEntries; i++ {
			target := filepath.Join(chunksFolderName, strconv.Itoa(i))
			err = cv.mount.Unlink(target)
			if err != nil {
				return
			}
		}
		_ = cv.mount.Unlink(chunksFolderName)
	})

	return true, chunk, nil
}

// WriteChunk saves an intermediate chunk temporarily and assembles all chunks
// once the final one is received.
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
