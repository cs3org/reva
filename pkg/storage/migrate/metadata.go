// Copyright 2018-2019 CERN
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

package migrate

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"path"
	"strconv"

	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
)

// metaData representation in the import data
type metaData struct {
	Type        string `json:"type"`
	Path        string `json:"path"`
	Etag        string `json:"eTag"`
	Permissions int    `json:"permissions"`
	MTime       int    `json:"mtime"`
}

//ImportMetadata from a files.jsonl file in exportPath. The files must already be present on the storage
//Will set etag and mtime
func ImportMetadata(ctx context.Context, client gatewayv0alphapb.GatewayServiceClient, exportPath string) error {

	filesJSONL, err := os.Open(path.Join(exportPath, "files.jsonl"))
	if err != nil {
		return err
	}
	defer filesJSONL.Close()
	jsonLines := bufio.NewScanner(filesJSONL)

	for jsonLines.Scan() {
		var fileData metaData
		if err := json.Unmarshal(jsonLines.Bytes(), &fileData); err != nil {
			log.Fatal(err)
			return err
		}

		m := make(map[string]string)
		if fileData.Etag != "" {
			// TODO sanitize etag? eg double quotes at beginning and end?
			m["etag"] = fileData.Etag
		}
		if fileData.MTime != 0 {
			m["mtime"] = strconv.Itoa(fileData.MTime)
		}
		//TODO permissions? is done via share? but this is owner permissions

		if len(m) > 0 {
			resourcePath := path.Join("/", path.Base(exportPath), fileData.Path)
			samReq := &storageproviderv0alphapb.SetArbitraryMetadataRequest{
				Ref: &storageproviderv0alphapb.Reference{
					Spec: &storageproviderv0alphapb.Reference_Path{Path: resourcePath},
				},
				ArbitraryMetadata: &storageproviderv0alphapb.ArbitraryMetadata{
					Metadata: m,
				},
			}
			samResp, err := client.SetArbitraryMetadata(ctx, samReq)

			if samResp.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
				log.Print("File does not exist on target system, skipping metadata import: " + resourcePath)
			} else if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Print("no etag or mtime for : " + fileData.Path)
		}

	}
	return nil
}
