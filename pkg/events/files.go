// Copyright 2018-2022 CERN
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

package events

import (
	"encoding/json"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// FileUploaded is emitted when a file is uploaded
type FileUploaded struct {
	Executant *user.UserId
	Ref       *provider.Reference
	Owner     *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (FileUploaded) Unmarshal(v []byte) (interface{}, error) {
	e := FileUploaded{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// FileDownloaded is emitted when a file is downloaded
type FileDownloaded struct {
	Ref   *provider.Reference
	Owner *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (FileDownloaded) Unmarshal(v []byte) (interface{}, error) {
	e := FileDownloaded{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// ItemTrashed is emitted when a file or folder is trashed
type ItemTrashed struct {
	Executant *user.UserId
	ID        *provider.ResourceId
	Ref       *provider.Reference
	Owner     *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (ItemTrashed) Unmarshal(v []byte) (interface{}, error) {
	e := ItemTrashed{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// ItemMoved is emitted when a file or folder is moved
type ItemMoved struct {
	Executant    *user.UserId
	Ref          *provider.Reference
	Owner        *user.UserId
	OldReference *provider.Reference
}

// Unmarshal to fulfill umarshaller interface
func (ItemMoved) Unmarshal(v []byte) (interface{}, error) {
	e := ItemMoved{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// ItemPurged is emitted when a file or folder is removed from trashbin
type ItemPurged struct {
	Executant *user.UserId
	ID        *provider.ResourceId
	Ref       *provider.Reference
	Owner     *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (ItemPurged) Unmarshal(v []byte) (interface{}, error) {
	e := ItemPurged{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// ItemRestored is emitted when a file or folder is restored from trashbin
type ItemRestored struct {
	Executant    *user.UserId
	ID           *provider.ResourceId
	Ref          *provider.Reference
	Owner        *user.UserId
	OldReference *provider.Reference
	Key          string
}

// Unmarshal to fulfill umarshaller interface
func (ItemRestored) Unmarshal(v []byte) (interface{}, error) {
	e := ItemRestored{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// FileVersionRestored is emitted when a file version is restored
type FileVersionRestored struct {
	Executant *user.UserId
	Ref       *provider.Reference
	Owner     *user.UserId
	Key       string
}

// Unmarshal to fulfill umarshaller interface
func (FileVersionRestored) Unmarshal(v []byte) (interface{}, error) {
	e := FileVersionRestored{}
	err := json.Unmarshal(v, &e)
	return e, err
}
