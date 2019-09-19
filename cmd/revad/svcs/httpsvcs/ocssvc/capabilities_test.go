package ocssvc

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"testing"
)

func TestMarshal(t *testing.T) {
	cd := CapabilitiesData{
		Capabilities: &Capabilities{
			FilesSharing: &CapabilitiesFilesSharing{
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
		fmt.Println(string(jsonData))
		fmt.Println(jsonExpect)
		t.Fail()
	}

	xmlData, err := xml.Marshal(&cd)
	if err != nil {
		t.Fail()
	}

	if string(xmlData) != xmlExpect {
		fmt.Println(string(xmlData))
		t.Fail()
	}
}
