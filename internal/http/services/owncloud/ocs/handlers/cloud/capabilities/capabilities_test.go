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

package capabilities

import (
	"encoding/json"
	"encoding/xml"
	"testing"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
)

func TestMarshal(t *testing.T) {
	cd := data.CapabilitiesData{
		Capabilities: &data.Capabilities{
			FilesSharing: &data.CapabilitiesFilesSharing{
				APIEnabled: true,
			},
		},
	}

	jsonExpect := `{"capabilities":{"core":null,"checksums":null,"files":null,"dav":null,"files_sharing":{"api_enabled":true,"resharing":false,"group_sharing":false,"auto_accept_share":false,"share_with_group_members_only":false,"share_with_membership_groups_only":false,"search_min_length":0,"default_permissions":0,"user_enumeration":null,"federation":null,"public":null,"user":null},"notifications":null},"version":null}`
	xmlExpect := `<CapabilitiesData><capabilities><files_sharing><api_enabled>1</api_enabled><resharing>0</resharing><group_sharing>0</group_sharing><auto_accept_share>0</auto_accept_share><share_with_group_members_only>0</share_with_group_members_only><share_with_membership_groups_only>0</share_with_membership_groups_only><search_min_length>0</search_min_length><default_permissions>0</default_permissions></files_sharing></capabilities></CapabilitiesData>`

	jsonData, err := json.Marshal(&cd)
	if err != nil {
		t.Fail()
	}

	if string(jsonData) != jsonExpect {
		t.Fail()
	}

	xmlData, err := xml.Marshal(&cd)
	if err != nil {
		t.Fail()
	}

	if string(xmlData) != xmlExpect {
		t.Fail()
	}
}
