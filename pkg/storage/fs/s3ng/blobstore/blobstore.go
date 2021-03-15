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
	"context"
	"io"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

// Blobstore provides an interface to an s3 compatible blobstore
type Blobstore struct {
	client *minio.Client

	bucket string
}

// New returns a new Blobstore
func New(endpoint, region, bucket, accessKey, secretKey string) (*Blobstore, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse s3 endpoint")
	}

	useSSL := u.Scheme != "http"
	client, err := minio.New(u.Host, &minio.Options{
		Region: region,
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup s3 client")
	}

	return &Blobstore{
		client: client,
		bucket: bucket,
	}, nil
}

// Upload stores some data in the blobstore under the given key
func (bs *Blobstore) Upload(key string, reader io.Reader) error {
	_, err := bs.client.PutObject(context.Background(), bs.bucket, key, reader, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})

	if err != nil {
		return errors.Wrapf(err, "could not store object '%s' into bucket '%s'", key, bs.bucket)
	}
	return nil
}

// Download retrieves a blob from the blobstore for reading
func (bs *Blobstore) Download(key string) (io.ReadCloser, error) {
	reader, err := bs.client.GetObject(context.Background(), bs.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not download object '%s' from bucket '%s'", key, bs.bucket)
	}
	return reader, nil
}

// Delete deletes a blob from the blobstore
func (bs *Blobstore) Delete(key string) error {
	err := bs.client.RemoveObject(context.Background(), bs.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not delete object '%s' from bucket '%s'", key, bs.bucket)
	}
	return nil
}
