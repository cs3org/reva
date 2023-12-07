// Copyright 2018-2023 CERN
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

package tus

import (
	"context"
	"fmt"
	"strings"

	tusd "github.com/tus/tusd/pkg/handler"
)

// DataStore is an interface that extends the tusd.DataStore interface.
type DataStore interface {
	tusd.DataStore
	NewUploadWithSession(ctx context.Context, session Session) (upload Upload, err error)
}

type Upload interface {
	tusd.Upload
	GetID() string
}

func BuildUploadId(spaceID, blobID string) string {
	return spaceID + ":" + blobID
}

func SplitUploadId(uploadID string) (spaceID string, blobID string, err error) {
	parts := strings.SplitN(uploadID, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid uploadid")
	}
	return parts[0], parts[1], nil
}
