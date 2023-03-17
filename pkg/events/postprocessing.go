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

type (
	// Postprocessingstep are the available postprocessingsteps
	Postprocessingstep string

	// PostprocessingOutcome defines the result of the postprocessing
	PostprocessingOutcome string
)

var (
	// PPStepAntivirus is the step that scans for viruses
	PPStepAntivirus Postprocessingstep = "virusscan"
	// PPStepPolicies is the step the step that enforces policies
	PPStepPolicies Postprocessingstep = "policies"
	// PPStepDelay is the step that processing. Useful for testing or user annoyment
	PPStepDelay Postprocessingstep = "delay"

	// PPOutcomeDelete means that the file and the upload should be deleted
	PPOutcomeDelete PostprocessingOutcome = "delete"
	// PPOutcomeAbort means that the upload is cancelled but the bytes are being kept in the upload folder
	PPOutcomeAbort PostprocessingOutcome = "abort"
	// PPOutcomeContinue means that the upload is moved to its final destination (eventually being marked with pp results)
	PPOutcomeContinue PostprocessingOutcome = "continue"
)

// BytesReceived is emitted by the server when it received all bytes of an upload
type BytesReceived struct {
	UploadID      string
	SpaceOwner    *user.UserId
	ExecutingUser *user.User
	ResourceID    *provider.ResourceId
	Filename      string
	Filesize      uint64
	URL           string
}

// Unmarshal to fulfill umarshaller interface
func (BytesReceived) Unmarshal(v []byte) (interface{}, error) {
	e := BytesReceived{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// StartPostprocessingStep can be issued by the server to start a postprocessing step
type StartPostprocessingStep struct {
	UploadID      string
	URL           string
	ExecutingUser *user.User
	Filename      string
	Filesize      uint64
	Token         string               // for file retrieval in after upload case
	ResourceID    *provider.ResourceId // for file retrieval in after upload case
	RevaToken     string               // for file retrieval in after upload case

	StepToStart Postprocessingstep
}

// Unmarshal to fulfill umarshaller interface
func (StartPostprocessingStep) Unmarshal(v []byte) (interface{}, error) {
	e := StartPostprocessingStep{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// PostprocessingStepFinished can be issued by the server when a postprocessing step is finished
type PostprocessingStepFinished struct {
	UploadID      string
	ExecutingUser *user.User
	Filename      string

	FinishedStep Postprocessingstep    // name of the step
	Result       interface{}           // result information see VirusscanResult for example
	Error        error                 // possible error of the step
	Outcome      PostprocessingOutcome // some services may cause postprocessing to stop
}

// Unmarshal to fulfill umarshaller interface
func (PostprocessingStepFinished) Unmarshal(v []byte) (interface{}, error) {
	e := PostprocessingStepFinished{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// VirusscanResult is the Result of a PostprocessingStepFinished event from the antivirus
type VirusscanResult struct {
	Infected    bool
	Description string
	Scandate    time.Time
	ResourceID  *provider.ResourceId
	ErrorMsg    string // empty when no error
}

// PostprocessingFinished is emitted by *some* service which can decide that
type PostprocessingFinished struct {
	UploadID      string
	Filename      string
	SpaceOwner    *user.UserId
	ExecutingUser *user.User
	Result        map[Postprocessingstep]interface{} // it is a map[step]Event
	Outcome       PostprocessingOutcome
}

// Unmarshal to fulfill umarshaller interface
func (PostprocessingFinished) Unmarshal(v []byte) (interface{}, error) {
	e := PostprocessingFinished{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// UploadReady is emitted by the storage provider when postprocessing is finished
type UploadReady struct {
	UploadID      string
	Filename      string
	SpaceOwner    *user.UserId
	ExecutingUser *user.User
	FileRef       *provider.Reference
	Failed        bool
	Timestamp     time.Time
	// add reference here? We could use it to inform client pp is finished
}

// Unmarshal to fulfill umarshaller interface
func (UploadReady) Unmarshal(v []byte) (interface{}, error) {
	e := UploadReady{}
	err := json.Unmarshal(v, &e)
	return e, err
}
