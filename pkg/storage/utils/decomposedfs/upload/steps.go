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
	"fmt"
	"os"
	"time"

	"github.com/cs3org/reva/v2/pkg/antivirus"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
)

// Initialize is the step that initializes the node
func Initialize(upload *Upload) postprocessing.Step {
	return postprocessing.NewStep("initialize", func() error {
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
func Assemble(upload *Upload, async bool, waitforscan bool) postprocessing.Step {
	requires := []string{"initialize"}
	if waitforscan {
		requires = append(requires, "scanning")
	}
	return postprocessing.NewStep("assembling", func() error {
		err := upload.finishUpload()
		// NOTE: this makes the testsuite happy - remove once adjusted
		if !async && upload.node != nil {
			_ = upload.node.UnmarkProcessing()
		}
		return err
	}, upload.cleanup, requires...)
}

// Scan scans the file for viruses
func Scan(upload *Upload, avType string) postprocessing.Step {
	return postprocessing.NewStep("scanning", func() error {
		// TODO: this races with assembling which deletes the binPath
		f, err := os.Open(upload.binPath)
		if err != nil {
			return err
		}

		scanner, err := antivirus.New(avType)
		if err != nil {
			return err
		}

		result, err := scanner.Scan(f)
		// TODO: what to do when the file is infected
		// TODO: what to do when there was an error while scanning?
		if err != nil {
			return err
		}

		s := ""
		if upload.node != nil {
			s = upload.node.InternalPath()
		}
		fmt.Printf("Scanning result(%s): %v %v\n", s, result, err)
		return nil
	}, nil, "initialize")
}

// Sleep just waits for the given time
func Sleep(_ *Upload, sleeptime time.Duration) postprocessing.Step {
	return postprocessing.NewStep("sleep", func() error {
		time.Sleep(sleeptime)
		return nil
	}, nil)
}
