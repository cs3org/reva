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

	"github.com/cs3org/reva/v2/pkg/owncloud/ocs"
)

func TestMarshal(t *testing.T) {
	cd := ocs.CapabilitiesData{
		Capabilities: &ocs.Capabilities{
			FilesSharing: &ocs.CapabilitiesFilesSharing{
				APIEnabled: true,
			},
		},
	}

	// TODO: remove resharing from these strings once web defaults to resharing=false
	jsonExpect := `{"capabilities":{"core":null,"checksums":null,"files":null,"dav":null,"files_sharing":{"api_enabled":true,"group_sharing":false,"sharing_roles":false,"deny_access":false,"auto_accept_share":false,"share_with_group_members_only":false,"share_with_membership_groups_only":false,"search_min_length":0,"default_permissions":0,"user_enumeration":null,"federation":null,"public":null,"user":null,"resharing":false}},"version":null}`
	xmlExpect := `<CapabilitiesData><capabilities><files_sharing><api_enabled>1</api_enabled><group_sharing>0</group_sharing><sharing_roles>0</sharing_roles><deny_access>0</deny_access><auto_accept_share>0</auto_accept_share><share_with_group_members_only>0</share_with_group_members_only><share_with_membership_groups_only>0</share_with_membership_groups_only><search_min_length>0</search_min_length><default_permissions>0</default_permissions><resharing>0</resharing></files_sharing></capabilities></CapabilitiesData>`

	jsonData, err := json.Marshal(&cd)
	if err != nil {
		t.Fatal("cant marshal json")
	}

	if string(jsonData) != jsonExpect {
		t.Log(string(jsonData))
		t.Fatal("json data does not match")
	}

	xmlData, err := xml.Marshal(&cd)
	if err != nil {
		t.Fatal("cant marshal xml")
	}

	if string(xmlData) != xmlExpect {
		t.Log(string(xmlData))
		t.Fatal("xml data does not match")
	}
}
