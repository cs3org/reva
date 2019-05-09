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

package s3

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cernbox/reva/pkg/appctx"
	"github.com/cernbox/reva/pkg/mime"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/fs/registry"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func init() {
	registry.Register("s3", New)
}

type config struct {
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Endpoint  string `mapstructure:"endpoint"`
	Bucket    string `mapstructure:"bucket"`
	Prefix    string `mapstructure:"prefix"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a s3 api.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	awsConfig := aws.NewConfig().
		WithHTTPClient(http.DefaultClient).
		WithMaxRetries(aws.UseServiceDefaultRetries).
		WithLogger(aws.NewDefaultLogger()).
		WithLogLevel(aws.LogOff).
		WithSleepDelay(time.Sleep).
		WithCredentials(credentials.NewStaticCredentials(c.AccessKey, c.SecretKey, "")).
		WithEndpoint(c.Endpoint).
		WithS3ForcePathStyle(true).
		WithDisableSSL(true)

	if c.Region != "" {
		awsConfig.WithRegion(c.Region)
	} else {
		awsConfig.WithRegion("us-east-1")
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, errors.New("creating the S3 session")
	}

	s3Client := s3.New(sess)

	return &s3FS{client: s3Client, config: c}, nil
}

func (fs *s3FS) Shutdown() error {
	return nil
}

func (fs *s3FS) addRoot(p string) string {
	np := path.Join(fs.config.Prefix, p)
	return np
}

func (fs *s3FS) removeRoot(np string) string {
	p := strings.TrimPrefix(np, fs.config.Prefix)
	if p == "" {
		p = "/"
	}
	return p
}

type s3FS struct {
	client *s3.S3
	config *config
}

func (fs *s3FS) normalizeObject(ctx context.Context, o *s3.Object, fn string) *storage.MD {
	fn = fs.removeRoot(path.Join("/", fn))
	isDir := strings.HasSuffix(*o.Key, "/")
	md := &storage.MD{
		ID:          "fileid-" + strings.TrimPrefix(fn, "/"),
		Path:        fn,
		IsDir:       isDir,
		Etag:        *o.ETag,
		Mime:        mime.Detect(isDir, fn),
		Permissions: &storage.PermissionSet{ListContainer: true, CreateContainer: true},
		Size:        uint64(*o.Size),
		Mtime: &storage.Timestamp{
			Seconds: uint64(o.LastModified.Unix()),
		},
	}
	appctx.GetLogger(ctx).Debug().
		Interface("object", o).
		Interface("metadata", md).
		Msg("normalized Object")
	return md
}
func (fs *s3FS) normalizeHead(ctx context.Context, o *s3.HeadObjectOutput, fn string) *storage.MD {
	fn = fs.removeRoot(path.Join("/", fn))
	isDir := strings.HasSuffix(fn, "/")
	md := &storage.MD{
		ID:          "fileid-" + strings.TrimPrefix(fn, "/"),
		Path:        fn,
		IsDir:       isDir,
		Etag:        *o.ETag,
		Mime:        mime.Detect(isDir, fn),
		Permissions: &storage.PermissionSet{ListContainer: true, CreateContainer: true},
		Size:        uint64(*o.ContentLength),
		Mtime: &storage.Timestamp{
			Seconds: uint64(o.LastModified.Unix()),
		},
	}
	appctx.GetLogger(ctx).Debug().
		Interface("head", o).
		Interface("metadata", md).
		Msg("normalized Head")
	return md
}
func (fs *s3FS) normalizeCommonPrefix(ctx context.Context, p *s3.CommonPrefix) *storage.MD {
	fn := fs.removeRoot(path.Join("/", *p.Prefix))
	md := &storage.MD{
		ID:          "fileid-" + strings.TrimPrefix(fn, "/"),
		Path:        fn,
		IsDir:       true,
		Etag:        "TODO",
		Mime:        mime.Detect(true, fn),
		Permissions: &storage.PermissionSet{ListContainer: true, CreateContainer: true},
		Size:        0,
		Mtime: &storage.Timestamp{
			Seconds: 0,
		},
	}
	appctx.GetLogger(ctx).Debug().
		Interface("prefix", p).
		Interface("metadata", md).
		Msg("normalized CommonPrefix")
	return md
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is that path of the file without the first slash
// thus the file id always points to the filename
func (fs *s3FS) GetPathByID(ctx context.Context, id string) (string, error) {
	return path.Join("/", strings.TrimPrefix(id, "fileid-")), nil
}

func (fs *s3FS) AddGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *s3FS) ListGrants(ctx context.Context, path string) ([]*storage.Grant, error) {
	return nil, notSupportedError("op not supported")
}

func (fs *s3FS) RemoveGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *s3FS) UpdateGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *s3FS) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *s3FS) CreateDir(ctx context.Context, fn string) error {
	log := appctx.GetLogger(ctx)
	fn = fs.addRoot(fn) + "/" // append / to indicate folder // TODO only if fn does not end in /

	input := &s3.PutObjectInput{
		Bucket:        aws.String(fs.config.Bucket),
		Key:           aws.String(fn),
		ContentType:   aws.String("application/octet-stream"),
		ContentLength: aws.Int64(0),
	}

	result, err := fs.client.PutObject(input)
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return notFoundError(fn)
			}
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "s3fs: error creating dir "+fn)
	}

	log.Debug().Interface("result", result) // todo cache etag?
	return nil
}

func (fs *s3FS) Delete(ctx context.Context, fn string) error {
	log := appctx.GetLogger(ctx)
	fn = fs.addRoot(fn)

	// first we need to find out if fn is a dir or a file

	_, err := fs.client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
	})
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
			case s3.ErrCodeNoSuchKey:
				return notFoundError(fn)
			}
		}
		// it might be a directory, so we can batch delete the prefix + /
		iter := s3manager.NewDeleteListIterator(fs.client, &s3.ListObjectsInput{
			Bucket: aws.String(fs.config.Bucket),
			Prefix: aws.String(fn + "/"),
		})
		batcher := s3manager.NewBatchDeleteWithClient(fs.client)
		if err := batcher.Delete(aws.BackgroundContext(), iter); err != nil {
			return err
		}
		// ok, we are done
		return nil
	}

	// we found an object, let's get rid of it
	result, err := fs.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
	})
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
			case s3.ErrCodeNoSuchKey:
				return notFoundError(fn)
			}
		}
		return errors.Wrap(err, "s3fs: error deleting "+fn)
	}

	log.Debug().Interface("result", result)
	return nil
}

func (fs *s3FS) moveObject(ctx context.Context, oldKey string, newKey string) error {

	// Copy
	// TODO double check CopyObject can deal with >5GB files.
	// Docs say we need to use multipart upload: https://docs.aws.amazon.com/AmazonS3/latest/API/RESTObjectCOPY.html
	_, err := fs.client.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(fs.config.Bucket),
		CopySource: aws.String("/" + fs.config.Bucket + oldKey),
		Key:        aws.String(newKey),
	})
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case s3.ErrCodeNoSuchBucket:
			return notFoundError(oldKey)
		}
		return err
	}
	// TODO cache etag and mtime?

	// Delete
	_, err = fs.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(oldKey),
	})
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case s3.ErrCodeNoSuchBucket:
		case s3.ErrCodeNoSuchKey:
			return notFoundError(oldKey)
		}
		return err
	}
	return nil
}

func (fs *s3FS) Move(ctx context.Context, oldName, newName string) error {
	log := appctx.GetLogger(ctx)
	fn := fs.addRoot(oldName)

	// first we need to find out if fn is a dir or a file

	_, err := fs.client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
	})
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
			case s3.ErrCodeNoSuchKey:
				return notFoundError(fn)
			}
		}

		// move directory
		input := &s3.ListObjectsV2Input{
			Bucket: aws.String(fs.config.Bucket),
			Prefix: aws.String(fn + "/"),
		}
		isTruncated := true

		for isTruncated {
			output, err := fs.client.ListObjectsV2(input)
			if err != nil {
				return errors.Wrap(err, "s3FS: error listing "+fn)
			}

			for _, o := range output.Contents {
				log.Debug().
					Interface("object", *o).
					Str("fn", fn).
					Msg("found Object")

				err := fs.moveObject(ctx, *o.Key, strings.Replace(*o.Key, fn+"/", newName+"/", 1))
				if err != nil {
					return err
				}
			}

			input.ContinuationToken = output.NextContinuationToken
			isTruncated = *output.IsTruncated
		}
		// ok, we are done
		return nil
	}

	// move single object
	err = fs.moveObject(ctx, fn, newName)
	if err != nil {
		return err
	}
	return nil
}

func (fs *s3FS) GetMD(ctx context.Context, fn string) (*storage.MD, error) {
	log := appctx.GetLogger(ctx)
	fn = fs.addRoot(fn)
	// first try a head, works for files
	log.Debug().
		Str("fn", fn).
		Msg("trying HEAD")

	input := &s3.HeadObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
	}
	output, err := fs.client.HeadObject(input)
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
			case s3.ErrCodeNoSuchKey:
				return nil, notFoundError(fn)
			}
		}
		log.Debug().
			Str("fn", fn).
			Msg("trying to list prefix")
		//try by listing parent to find directory
		input := &s3.ListObjectsV2Input{
			Bucket:    aws.String(fs.config.Bucket),
			Prefix:    aws.String(fn),
			Delimiter: aws.String("/"), // limit to a single directory
		}
		isTruncated := true

		for isTruncated {
			output, err := fs.client.ListObjectsV2(input)
			if err != nil {
				return nil, errors.Wrap(err, "s3FS: error listing "+fn)
			}

			for _, o := range output.CommonPrefixes {
				log.Debug().
					Interface("object", *o).
					Str("fn", fn).
					Msg("found CommonPrefix")
				if *o.Prefix == fn+"/" {
					return fs.normalizeCommonPrefix(ctx, o), nil
				}
			}

			input.ContinuationToken = output.NextContinuationToken
			isTruncated = *output.IsTruncated
		}
		return nil, notFoundError(fn)
	}

	return fs.normalizeHead(ctx, output, fn), nil
}

func (fs *s3FS) ListFolder(ctx context.Context, fn string) ([]*storage.MD, error) {
	fn = fs.addRoot(fn)

	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(fs.config.Bucket),
		Prefix:    aws.String(fn + "/"),
		Delimiter: aws.String("/"), // limit to a single directory
	}
	isTruncated := true

	finfos := []*storage.MD{}

	for isTruncated {
		output, err := fs.client.ListObjectsV2(input)
		if err != nil {
			return nil, errors.Wrap(err, "s3FS: error listing "+fn)
		}

		for _, p := range output.CommonPrefixes {
			finfos = append(finfos, fs.normalizeCommonPrefix(ctx, p))
		}

		for _, o := range output.Contents {
			finfos = append(finfos, fs.normalizeObject(ctx, o, *o.Key))
		}

		input.ContinuationToken = output.NextContinuationToken
		isTruncated = *output.IsTruncated
	}
	// TODO sort fileinfos?
	return finfos, nil
}

func (fs *s3FS) Upload(ctx context.Context, fn string, r io.ReadCloser) error {
	log := appctx.GetLogger(ctx)
	fn = fs.addRoot(fn)

	upParams := &s3manager.UploadInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
		Body:   r,
	}
	uploader := s3manager.NewUploaderWithClient(fs.client)
	result, err := uploader.Upload(upParams)

	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return notFoundError(fn)
			}
		}
		return errors.Wrap(err, "s3fs: error creating object "+fn)
	}

	log.Debug().Interface("result", result) // todo cache etag?
	return nil
}

func (fs *s3FS) Download(ctx context.Context, fn string) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	fn = fs.addRoot(fn)

	// use GetObject instead of s3manager.Downloader:
	// the result.Body is a ReadCloser, which allows streaming
	// TODO double check we are not caching bytes in memory
	r, err := fs.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(fs.config.Bucket),
		Key:    aws.String(fn),
	})
	if err != nil {
		log.Error().Err(err)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
			case s3.ErrCodeNoSuchKey:
				return nil, notFoundError(fn)
			}
		}
		return nil, errors.Wrap(err, "s3fs: error deleting "+fn)
	}
	return r.Body, nil
}

func (fs *s3FS) ListRevisions(ctx context.Context, path string) ([]*storage.Revision, error) {
	return nil, notSupportedError("list revisions")
}

func (fs *s3FS) DownloadRevision(ctx context.Context, path, revisionKey string) (io.ReadCloser, error) {
	return nil, notSupportedError("download revision")
}

func (fs *s3FS) RestoreRevision(ctx context.Context, path, revisionKey string) error {
	return notSupportedError("restore revision")
}

func (fs *s3FS) EmptyRecycle(ctx context.Context, path string) error {
	return notSupportedError("empty recycle")
}

func (fs *s3FS) ListRecycle(ctx context.Context, path string) ([]*storage.RecycleItem, error) {
	return nil, notSupportedError("list recycle")
}

func (fs *s3FS) RestoreRecycleItem(ctx context.Context, fn, restoreKey string) error {
	return notSupportedError("restore recycle")
}

type notSupportedError string
type notFoundError string

func (e notSupportedError) Error() string   { return string(e) }
func (e notSupportedError) IsNotSupported() {}
func (e notFoundError) Error() string       { return string(e) }
func (e notFoundError) IsNotFound()         {}
