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

package blobstore

import (
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
)

// Blobstore provides an interface to an s3 compatible blobstore
type Blobstore struct {
	uploader *s3manager.Uploader
	bucket   string
}

// New returns a new Blobstore
func New(endpoint, region, bucket, accessKey, secretKey string) (*Blobstore, error) {
	sess, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup s3 session")
	}
	uploader := s3manager.NewUploader(sess)

	return &Blobstore{
		uploader: uploader,
	}, nil
}

// Upload stores some data in the blobstore under the given key
func (bs *Blobstore) Upload(key string, reader io.Reader) error {
	_, err := bs.uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bs.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return errors.Wrapf(err, "could not store object: %s", key)
	}
	return nil
}
