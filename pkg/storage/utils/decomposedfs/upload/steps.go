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
	"os"
	"time"

	"github.com/cs3org/reva/v2/pkg/antivirus"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
	"github.com/pkg/errors"
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
func Assemble(upload *Upload, async bool, waitforscan bool) postprocessing.Step {
	requires := []string{"initialize"}
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

// Scan scans the file for viruses
func Scan(upload *Upload, avType string, handle string) postprocessing.Step {
	return postprocessing.NewStep("scanning", func() error {
		if upload.Cancelled.IsTrue() {
			return nil
		}

		f, err := os.Open(upload.binPath)
		if err != nil {
			return err
		}

		scanner, err := antivirus.New(avType)
		if err != nil {
			return err
		}

		result, err := scanner.Scan(f)
		if err != nil {
			// What to do when there was an error while scanning? -> file should stay in uploadpath for now
			upload.Cancelled.True()
			return err
		}

		if err := upload.node.SetScanData(result.Description); err != nil {
			// What to do? scan is done but we can't write the info to the node
			return err
		}

		if !result.Infected {
			// all good
			return nil
		}

		// TODO: send email that file was infected

		switch options.InfectedFileOption(handle) {
		default:
			// NOTE: we secretly default to delete
			fallthrough
		case options.Delete:
			upload.cleanup(true, true, true)
			upload.Cancelled.True()
			return nil
		case options.Keep:
			upload.Cancelled.True()
			return nil
		case options.Error:
			return errors.New("file infected")
		case options.Ignore:
			return nil
		}

	}, nil, "initialize")
}

// Sleep just waits for the given time
func Sleep(_ *Upload, sleeptime time.Duration) postprocessing.Step {
	return postprocessing.NewStep("sleep", func() error {
		time.Sleep(sleeptime)
		return nil
	}, nil)
}
