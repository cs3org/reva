// Copyright 2018-2021 CERN
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

package main

import (
	"fmt"

	"github.com/cs3org/cato"
	"github.com/cs3org/cato/resources"
)

func main() {
	rootPath := "."
	conf := &resources.CatoConfig{
		Driver: "reva",
		DriverConfig: map[string]map[string]interface{}{
			"reva": map[string]interface{}{
				"DocPaths": map[string]string{
					"internal/": "docs/content/en/docs/config/",
					"pkg/":      "docs/content/en/docs/config/packages/",
				},
				"ReferenceBase": "https://github.com/cs3org/reva/tree/master",
			},
		},
	}
	if _, err := cato.GenerateDocumentation(rootPath, conf); err != nil {
		fmt.Println("Error: ", err.Error())
	}
}
