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
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// BytesReceived is emitted by the server when it received all bytes of an upload
type BytesReceived struct {
	UploadID  string
	UploadURL string
	Token     string
}

// Unmarshal to fulfill umarshaller interface
func (BytesReceived) Unmarshal(v []byte) (interface{}, error) {
	e := BytesReceived{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// VirusscanFinished is emitted by the server when it has completed an antivirus scan
type VirusscanFinished struct {
	UploadID    string
	Infected    bool
	Description string
	Scandate    time.Time
}

// Unmarshal to fulfill umarshaller interface
func (VirusscanFinished) Unmarshal(v []byte) (interface{}, error) {
	e := VirusscanFinished{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// StartPostprocessingStep can be issued by the server to start a postprocessing step
type StartPostprocessingStep struct {
	UploadID  string
	UploadURL string
	Token     string

	StepToStart string
}

// Unmarshal to fulfill umarshaller interface
func (StartPostprocessingStep) Unmarshal(v []byte) (interface{}, error) {
	e := StartPostprocessingStep{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// TODO: implement PostprocessingStepFinished event to enable client to track pp status

// PostprocessingFinished is emitted by *some* service which can decide that
type PostprocessingFinished struct {
	UploadID string
	Result   map[string]interface{} // it is basically a map[step]Event
	Action   string                 // "delete", "cancel" or "continue"
}

// Unmarshal to fulfill umarshaller interface
func (PostprocessingFinished) Unmarshal(v []byte) (interface{}, error) {
	e := PostprocessingFinished{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// UploadReady is emitted by the storage provider when postprocessing is finished and the file is ready to work with
type UploadReady struct {
	UploadID string
	// add reference here? We could use it to inform client pp is finished
}

// Unmarshal to fulfill umarshaller interface
func (UploadReady) Unmarshal(v []byte) (interface{}, error) {
	e := UploadReady{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// ContainerCreated is emitted when a directory has been created
type ContainerCreated struct {
	Executant *user.UserId
	Ref       *provider.Reference
	Owner     *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (ContainerCreated) Unmarshal(v []byte) (interface{}, error) {
	e := ContainerCreated{}
	err := json.Unmarshal(v, &e)
	return e, err
}

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

// FileTouched is emitted when a file is uploaded
type FileTouched struct {
	Executant *user.UserId
	Ref       *provider.Reference
}

// Unmarshal to fulfill umarshaller interface
func (FileTouched) Unmarshal(v []byte) (interface{}, error) {
	e := FileTouched{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// FileDownloaded is emitted when a file is downloaded
type FileDownloaded struct {
	Executant *user.UserId
	Ref       *provider.Reference
	Owner     *user.UserId
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
