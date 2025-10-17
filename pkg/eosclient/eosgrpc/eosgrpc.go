// Copyright 2018-2024 CERN
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

// NOTE: compile the grpc proto with these commands
// and do not ask any questions, I don't have the answer
// protoc ./Rpc.proto --go_out=plugins=grpc:.

package eosgrpc

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// SystemAttr is the system extended attribute.
	SystemAttr eosclient.AttrType = iota
	// UserAttr is the user extended attribute.
	UserAttr
)

// Client performs actions against a EOS management node (MGM)
// using the EOS GRPC interface.
type Client struct {
	opt    *Options
	httpcl *EOSHTTPClient
	cl     erpc.EosClient
}

// Options to configure the Client.
type Options struct {
	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool

	// Whether to maintain the same inode across various versions of a file.
	// Requires extra metadata operations if set to true
	VersionInvariant bool

	// Set to true to use the local disk as a buffer for chunk
	// reads from EOS. Default is false, i.e. pure streaming
	ReadUsesLocalTemp bool

	// Set to true to use the local disk as a buffer for chunk
	// writes to EOS. Default is false, i.e. pure streaming
	// Beware: in pure streaming mode the FST must support
	// the HTTP chunked encoding
	WriteUsesLocalTemp bool

	// Location of the xrdcopy binary.
	// Default is /opt/eos/xrootd/bin/xrdcopy.
	XrdcopyBinary string

	// URL of the EOS MGM.
	// Default is root://eos-example.org
	URL string

	// URI of the EOS MGM grpc server
	GrpcURI string

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string

	// Keytab is the location of the EOS keytab file.
	Keytab string

	// Authkey is the key that authorizes this client to connect to the GRPC service
	Authkey string

	// SecProtocol is the comma separated list of security protocols used by xrootd.
	// For example: "sss, unix"
	SecProtocol string

	// TokenExpiry stores in seconds the time after which generated tokens will expire
	// Default is 3600
	TokenExpiry int

	// AllowInsecure determines whether EOS can fall back to no TLS
	// Default is false
	AllowInsecure bool
}

func (opt *Options) init() {
	if opt.XrdcopyBinary == "" {
		opt.XrdcopyBinary = "/opt/eos/xrootd/bin/xrdcopy"
	}

	if opt.URL == "" {
		opt.URL = "root://eos-example.org"
	}

	if opt.CacheDirectory == "" {
		opt.CacheDirectory = os.TempDir()
	}
}

// Create and connect a grpc eos Client.
func newgrpc(ctx context.Context, log *zerolog.Logger, opt *Options) (erpc.EosClient, error) {
	log.Debug().Msgf("Setting up GRPC towards '%s'", opt.GrpcURI)

	certpool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(opt.GrpcURI, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certpool, "")))
	if err != nil {
		log.Warn().Err(err).Msgf("Error connecting to '%s' using TLS", opt.GrpcURI)
		return nil, err
	}

	log.Debug().Msgf("Going to ping '%s'", opt.GrpcURI)
	ecl := erpc.NewEosClient(conn)
	// If we can't ping... just print warnings. In the case EOS is down, grpc will take care of
	// connecting later
	prq := new(erpc.PingRequest)
	prq.Authkey = opt.Authkey
	prq.Message = []byte("hi this is a ping from reva")
	prep, err := ecl.Ping(ctx, prq)
	if err != nil {
		if opt.AllowInsecure {
			conn, err = grpc.NewClient(opt.GrpcURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Warn().Err(err).Msgf("Error connecting to '%s' using insecure", opt.GrpcURI)
				return nil, err
			} else {
				log.Warn().Err(err).Msgf("Fell back to insecure mode when connecting to %s because TLS ping failed", opt.GrpcURI)
				ecl = erpc.NewEosClient(conn)
			}
		} else {
			log.Error().Err(err).Msgf("Failed to connect to '%s' using TLS, and allow_insecure is false", opt.GrpcURI)
			return nil, err
		}
		log.Warn().Err(err).Msgf("Could not ping to '%s'", opt.GrpcURI)
	}

	if prep == nil {
		log.Warn().Msgf("Could not ping to '%s': nil response", opt.GrpcURI)
	}
	log.Debug().Msgf("Ping to '%s' succeeded", opt.GrpcURI)

	return ecl, nil
}

// New creates a new client with the given options.
func New(ctx context.Context, opt *Options, httpOpts *HTTPOptions) (*Client, error) {
	log := appctx.GetLogger(ctx)

	log.Debug().Interface("options", opt).Msgf("Creating new eosgrpc client")

	opt.init()
	httpcl, err := NewEOSHTTPClient(httpOpts)
	if err != nil {
		return nil, err
	}

	cl, err := newgrpc(ctx, log, opt)
	if err != nil {
		return nil, err
	}

	return &Client{
		opt:    opt,
		httpcl: httpcl,
		cl:     cl,
	}, nil
}

// Common code to create and initialize a NSRequest.
func (c *Client) initNSRequest(ctx context.Context, auth eosclient.Authorization, app string) (*erpc.NSRequest, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("(uid,gid)", "("+auth.Role.UID+","+auth.Role.GID+")").Str("app", app).Msg("New grpcNS req")

	rq := new(erpc.NSRequest)
	rq.Role = new(erpc.RoleId)

	// Let's put in the authentication info
	if auth.Token != "" {
		// Map to owner using EOSAUTHZ token
		// We do not become cbox
		rq.Authkey = auth.Token
	} else {
		// We take the secret key from the config, which maps on EOS to cbox
		// cbox is a sudo'er, so we become the user specified in UID/GID, if it is set
		rq.Authkey = c.opt.Authkey

		uid, gid, err := utils.ExtractUidGid(auth)
		if err == nil {
			rq.Role.Uid = uid
			rq.Role.Gid = gid
		}
	}

	// For NS operations, specifically for locking, we also need to provide the app
	rq.Role.App = app

	rq.Role.Trace = trace.Get(ctx)

	return rq, nil
}

// Common code to create and initialize a MDRequest.
func (c *Client) initMDRequest(ctx context.Context, auth eosclient.Authorization) (*erpc.MDRequest, error) {
	// Stuff filename, uid, gid into the MDRequest type

	log := appctx.GetLogger(ctx)
	log.Debug().Str("(uid,gid)", "("+auth.Role.UID+","+auth.Role.GID+")").Msg("New grpcMD req")

	rq := new(erpc.MDRequest)
	rq.Role = new(erpc.RoleId)

	if auth.Token != "" {
		// Map to owner using EOSAUTHZ token
		// We do not become cbox
		rq.Authkey = auth.Token
	} else {
		// We take the secret key from the config, which maps on EOS to cbox
		// cbox is a sudo'er, so we become the user specified in UID/GID, if it is set
		rq.Authkey = c.opt.Authkey

		uid, gid, err := utils.ExtractUidGid(auth)
		if err == nil {
			rq.Role.Uid = uid
			rq.Role.Gid = gid
		}
	}

	rq.Role.Trace = trace.Get(ctx)

	return rq, nil
}

// Read reads a file from the mgm and returns a handle to read it
// This handle could be directly the body of the response or a local tmp file
//
//	returning a handle to the body is nice, yet it gives less control on the transaction
//	itself, e.g. strange timeouts or TCP issues may be more difficult to trace
//
// Let's consider this experimental for the moment, maybe I'll like to add a config
// parameter to choose between the two behaviours.
func (c *Client) Read(ctx context.Context, auth eosclient.Authorization, path string, ranges []storage.Range) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Read").Any("Ranges", ranges).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	var localTarget string
	var err error
	var localfile io.WriteCloser
	localfile = nil

	u, err := utils.GetUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eos: no user in ctx")
	}

	if c.opt.ReadUsesLocalTemp {
		rand := "eosread-" + uuid.New().String()
		localTarget := fmt.Sprintf("%s/%s", c.opt.CacheDirectory, rand)
		defer os.RemoveAll(localTarget)

		log.Info().Str("func", "Read").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("tempfile", localTarget).Msg("")
		localfile, err = os.Create(localTarget)
		if err != nil {
			log.Error().Str("func", "Read").Str("path", path).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("err", err.Error()).Msg("")
			return nil, errtypes.InternalError(fmt.Sprintf("can't open local temp file '%s'", localTarget))
		}
	}

	bodystream, err := c.httpcl.GETFile(ctx, u.Username, auth, path, localfile, ranges)
	if err != nil {
		log.Error().Str("func", "Read").Str("path", path).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("err", err.Error()).Msg("")
		return nil, errtypes.InternalError(fmt.Sprintf("can't GET local cache file '%s'", localTarget))
	}

	return bodystream, nil
	// return os.Open(localTarget)
}

// Write writes a file to the mgm
// Somehow the same considerations as Read apply.
func (c *Client) Write(ctx context.Context, auth eosclient.Authorization, path string, stream io.ReadCloser, length int64, app string, disableVersioning bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Write").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Msg("")

	u, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eos: no user in ctx")
	}

	if c.opt.WriteUsesLocalTemp {
		fd, err := os.CreateTemp(c.opt.CacheDirectory, "eoswrite-")
		if err != nil {
			return err
		}
		defer fd.Close()
		defer os.RemoveAll(fd.Name())

		log.Info().Str("func", "Write").Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", path).Str("tempfile", fd.Name()).Msg("")
		// copy stream to local temp file
		length, err = io.Copy(fd, stream)
		if err != nil {
			return err
		}

		wfd, err := os.Open(fd.Name())
		if err != nil {
			return err
		}
		defer wfd.Close()
		defer os.RemoveAll(fd.Name())

		return c.httpcl.PUTFile(ctx, u.Username, auth, path, wfd, length, app, disableVersioning)
	}

	return c.httpcl.PUTFile(ctx, u.Username, auth, path, stream, length, app, disableVersioning)
}

func (c *Client) getOrCreateVersionFolderInode(ctx context.Context, ownerAuth eosclient.Authorization, p string) (uint64, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "getOrCreateVersionFolderInode").Str("uid,gid", ownerAuth.Role.UID+","+ownerAuth.Role.GID).Str("p", p).Msg("")

	if eosclient.IsVersionFolder(filepath.Dir(p)) {
		log.Error().Str("path", p).Msg("getOrCreateVersionFolderInode called on version file!")
		return 0, errors.New("cannot get version folder of version file")
	}

	versionFolder := eosclient.GetVersionFolder(p)
	md, err := c.GetFileInfoByPath(ctx, ownerAuth, versionFolder)
	if err != nil {
		if err = c.CreateDir(ctx, ownerAuth, versionFolder); err != nil {
			return 0, err
		}
		md, err = c.GetFileInfoByPath(ctx, ownerAuth, versionFolder)
		if err != nil {
			return 0, err
		}
	}
	return md.Inode, nil
}
