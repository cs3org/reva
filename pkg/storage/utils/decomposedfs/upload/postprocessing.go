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

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
)

// Postprocessing will configure a postprocessing instance from config
func Postprocessing(upload *Upload, o options.PostprocessingOptions) postprocessing.Postprocessing {
	//waitfor := []string{"initialize"}
	waitfor := []string{}
	if !o.AsyncFileUploads {
		waitfor = append(waitfor, "assembling")
	}

	return postprocessing.Postprocessing{
		Steps:   stepsFromConfig(upload, o),
		WaitFor: waitfor,
		Finish:  finishPostprocessing(upload, o),
	}
}

func stepsFromConfig(upload *Upload, o options.PostprocessingOptions) []postprocessing.Step {
	steps := []postprocessing.Step{Initialize(upload)}
	if o.DelayProcessing != 0 {
		steps = append(steps, Sleep(upload, o.DelayProcessing))
	}

	steps = append(steps, Assemble(upload, o.AsyncFileUploads, false, o.DelayProcessing != 0))

	return steps
}

func finishPostprocessing(upload *Upload, o options.PostprocessingOptions) func(map[string]error) {
	return func(m map[string]error) {

		var failure bool
		for alias, err := range m {
			if err != nil {
				upload.log.Info().Str("ID", upload.Info.ID).Str("step", alias).Err(err).Msg("postprocessing failed")

				// NOTE: not all errors might be critical failures - use alias to determine which are not
				failure = true
			}
		}

		cancelled := upload.Cancelled.IsTrue()
		removeNode := failure || cancelled // remove node when upload failed or upload is cancelled
		removeBin := !cancelled            // remove bin & info when upload wasn't cancelled
		upload.cleanup(removeNode, removeBin, removeBin)

		if upload.node == nil {
			return
		}

		// unset processing status
		if err := upload.node.UnmarkProcessing(); err != nil {
			upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("unmarking processing failed")
		}

		if failure || cancelled {
			return
		}

		if o.AsyncFileUploads { // updating the mtime will cause the testsuite to fail - hence we do it only in async case
			now := utils.TSNow()
			if err := upload.node.SetMtime(upload.Ctx, fmt.Sprintf("%d.%d", now.Seconds, now.Nanos)); err != nil {
				upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("could not set mtime")
			}
		}

		if err := upload.tp.Propagate(upload.Ctx, upload.node); err != nil {
			upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("could not set mtime")
		}
	}

}
