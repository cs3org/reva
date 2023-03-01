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

package backend

import (
	"github.com/pkg/errors"
)

var errUnconfiguredError = errors.New("no xattrs backend configured. Bailing out")

// Backend defines the interface for file attribute backends
type Backend interface {
	All(path string) (map[string]string, error)
	Get(path, key string) (string, error)
	GetInt64(path, key string) (int64, error)
	List(path string) (attribs []string, err error)
	Set(path, key, val string) error
	SetMultiple(path string, attribs map[string]string, acquireLock bool) error
	Remove(path, key string) error

	Purge(path string) error
	Rename(oldPath, newPath string) error
	IsMetaFile(path string) bool
	MetadataPath(path string) string
}

// NullBackend is the default stub backend, used to enforce the configuration of a proper backend
type NullBackend struct{}

// All reads all extended attributes for a node
func (NullBackend) All(path string) (map[string]string, error) { return nil, errUnconfiguredError }

// Get an extended attribute value for the given key
func (NullBackend) Get(path, key string) (string, error) { return "", errUnconfiguredError }

// GetInt64 reads a string as int64 from the xattrs
func (NullBackend) GetInt64(path, key string) (int64, error) { return 0, errUnconfiguredError }

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (NullBackend) List(path string) ([]string, error) { return nil, errUnconfiguredError }

// Set sets one attribute for the given path
func (NullBackend) Set(path string, key string, val string) error { return errUnconfiguredError }

// SetMultiple sets a set of attribute for the given path
func (NullBackend) SetMultiple(path string, attribs map[string]string, acquireLock bool) error {
	return errUnconfiguredError
}

// Remove removes an extended attribute key
func (NullBackend) Remove(path string, key string) error { return errUnconfiguredError }

// IsMetaFile returns whether the given path represents a meta file
func (NullBackend) IsMetaFile(path string) bool { return false }

// Purge purges the data of a given path from any cache that might hold it
func (NullBackend) Purge(purges string) error { return errUnconfiguredError }

// Rename moves the data for a given path to a new path
func (NullBackend) Rename(oldPath, newPath string) error { return errUnconfiguredError }

// MetadataPath returns the path of the file holding the metadata for the given path
func (NullBackend) MetadataPath(path string) string { return "" }
