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

package upload

import (
	"time"

	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
)

// Initialize is the step that initializes the node
func Initialize(upload *Upload) postprocessing.Step {
	return postprocessing.NewStep("initialize", func() error {
		if upload.Cancelled.IsTrue() {
			return nil
		}
		// we need the node to start processing
		n, err := CreateNodeForUpload(upload)
		if err != nil {
			return err
		}

		// set processing status
		upload.node = n
		return upload.node.MarkProcessing()
	}, nil)
}

// Assemble assembles the file and moves it to the blobstore
func Assemble(upload *Upload, async bool, waitforscan bool, delayprocessing bool) postprocessing.Step {
	requires := []string{"initialize"}
	if delayprocessing {
		requires = append(requires, "sleep")
	}
	if waitforscan {
		requires = append(requires, "scanning")
	}
	return postprocessing.NewStep("assembling", func() error {
		if upload.Cancelled.IsTrue() {
			return nil
		}

		err := upload.finishUpload()
		if !async && upload.node != nil {
			_ = upload.node.UnmarkProcessing() // NOTE: this makes the testsuite happy - remove once adjusted
		}
		return err
	}, nil, requires...)
}

// Sleep just waits for the given time
func Sleep(_ *Upload, sleeptime time.Duration) postprocessing.Step {
	return postprocessing.NewStep("sleep", func() error {
		time.Sleep(sleeptime)
		return nil
	}, nil)
}
