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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	tuss3store "github.com/tus/tusd/pkg/s3store"
)

type S3Store struct {
	tuss3store.S3Store
}

func NewS3Store(bucket string, service tuss3store.S3API) S3Store {
	return S3Store{tuss3store.New(bucket, service)}
}

// CleanupMetadata removes the metadata associated with the given ID.
func (store S3Store) CleanupMetadata(ctx context.Context, id string) error {
	uploadID, _ := splitIds(id)
	_, err := store.Service.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(store.Bucket),
		Key:    store.metadataKeyWithPrefix(uploadID + ".info"),
	})
	return err
}

func (store S3Store) metadataKeyWithPrefix(key string) *string {
	prefix := store.MetadataObjectPrefix
	if prefix == "" {
		prefix = store.ObjectPrefix
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return aws.String(prefix + key)
}

func splitIds(id string) (uploadId, multipartId string) {
	index := strings.Index(id, "+")
	if index == -1 {
		return
	}

	uploadId = id[:index]
	multipartId = id[index+1:]
	return
}
