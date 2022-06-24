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

import "github.com/cs3org/reva/v2/pkg/utils/postprocessing"

func configurePostprocessing(upload *Upload) postprocessing.Postprocessing {
	// TODO: make configurable
	return postprocessing.Postprocessing{
		Steps: []postprocessing.Step{
			postprocessing.NewStep("initialize", func() error {
				// we need the node to start processing
				n, err := CreateNodeForUpload(upload)
				if err != nil {
					return err
				}

				// set processing status
				upload.node = n
				return upload.node.SetMetadata("user.ocis.nodestatus", "processing")
			}, nil),
			postprocessing.NewStep("assembling", upload.finishUpload, upload.cleanup, "initialize"),
		},
		WaitFor: []string{"assembling"}, // needed for testsuite atm, see comment in upload.cleanup
		Finish: func(_ map[string]error) {
			// TODO: Handle postprocessing errors

			if upload.node != nil {
				// temp if to lock marie in eternal processing - dont merge with this
				if upload.node.SpaceID == "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c" && upload.node.SpaceID != upload.node.ID {
					return
				}
				// unset processing status
				_ = upload.node.RemoveMetadata("user.ocis.nodestatus")
			}
		},
	}
}
