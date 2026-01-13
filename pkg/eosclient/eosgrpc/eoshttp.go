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

package eosgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/eosclient"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// HTTPOptions to configure the Client.
type HTTPOptions struct {

	// HTTP URL of the EOS MGM.
	// Default is https://eos-example.org
	BaseURL string

	// Timeout in seconds for connecting to the service
	ConnectTimeout int

	// Timeout in seconds for sending a request to the service and getting a response
	// Does not include redirections
	RWTimeout int

	// Timeout in seconds for performing an operation. Includes every redirection, retry, etc
	OpTimeout int

	// Max idle conns per Transport
	MaxIdleConns int

	// Max conns per transport per destination host
	MaxConnsPerHost int

	// Max idle conns per transport per destination host
	MaxIdleConnsPerHost int

	// TTL for an idle conn per transport
	IdleConnTimeout int

	// If the URL is https, then we need to configure this client
	// with the usual TLS stuff
	// Defaults are /etc/grid-security/hostcert.pem and /etc/grid-security/hostkey.pem
	ClientCertFile string
	ClientKeyFile  string

	// These will override the defaults, which are common system paths hardcoded
	// in the go x509 implementation (why did they do that?!?!?)
	// of course /etc/grid-security/certificates is NOT in those defaults!
	ClientCADirs  string
	ClientCAFiles string

	// Authkey is the key that authorizes this client to connect to the EOS HTTP service
	Authkey string
}

// Init fills the basic fields.
func (opt *HTTPOptions) init() {
	if opt.BaseURL == "" {
		opt.BaseURL = "https://eos-example.org"
	}

	if opt.ConnectTimeout == 0 {
		opt.ConnectTimeout = 30
	}
	if opt.RWTimeout == 0 {
		opt.RWTimeout = 180
	}
	if opt.OpTimeout == 0 {
		opt.OpTimeout = 360
	}
	if opt.MaxIdleConns == 0 {
		opt.MaxIdleConns = 100
	}
	if opt.MaxConnsPerHost == 0 {
		opt.MaxConnsPerHost = 64
	}
	if opt.MaxIdleConnsPerHost == 0 {
		opt.MaxIdleConnsPerHost = 8
	}
	if opt.IdleConnTimeout == 0 {
		opt.IdleConnTimeout = 30
	}

	if opt.ClientCertFile == "" {
		opt.ClientCertFile = "/etc/grid-security/hostcert.pem"
	}
	if opt.ClientKeyFile == "" {
		opt.ClientKeyFile = "/etc/grid-security/hostkey.pem"
	}

	if opt.ClientCAFiles != "" {
		os.Setenv("SSL_CERT_FILE", opt.ClientCAFiles)
	}
	if opt.ClientCADirs != "" {
		os.Setenv("SSL_CERT_DIR", opt.ClientCADirs)
	} else {
		os.Setenv("SSL_CERT_DIR", "/etc/grid-security/certificates")
	}
}

// EOSHTTPClient performs HTTP-based tasks (e.g. upload, download)
// against a EOS management node (MGM)
// using the EOS XrdHTTP interface.
// In this module we wrap eos-related behaviour, e.g. headers or r/w retries.
type EOSHTTPClient struct {
	opt *HTTPOptions
	cl  *http.Client
}

// NewEOSHTTPClient creates a new client with the given options.
func NewEOSHTTPClient(opt *HTTPOptions) (*EOSHTTPClient, error) {
	if opt == nil {
		return nil, errtypes.InternalError("HTTPOptions is nil")
	}

	opt.init()
	t := &http.Transport{
		MaxIdleConns:        opt.MaxIdleConns,
		MaxConnsPerHost:     opt.MaxConnsPerHost,
		MaxIdleConnsPerHost: opt.MaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(opt.IdleConnTimeout) * time.Second,
		DisableCompression:  true,
	}

	cl := &http.Client{
		Transport: t,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &EOSHTTPClient{
		opt: opt,
		cl:  cl,
	}, nil
}

// Format a human readable line that describes a response.
func rspdesc(rsp *http.Response) string {
	desc := "'" + fmt.Sprintf("%d", rsp.StatusCode) + "'" + ": '" + rsp.Status + "'"

	buf := new(bytes.Buffer)
	r := "<none>"
	n, e := buf.ReadFrom(rsp.Body)

	if e != nil {
		r = "Error reading body: '" + e.Error() + "'"
	} else if n > 0 {
		r = buf.String()
	}

	desc += " - '" + r + "'"

	return desc
}

func (c *EOSHTTPClient) doReq(req *http.Request, remoteuser string) (*http.Response, error) {
	// Here we put the headers that are required by EOS >= 5
	req.Header.Set("x-gateway-authorization", c.opt.Authkey)
	req.Header.Set("x-forwarded-for", "dummy")
	req.Header.Set("remote-user", remoteuser)

	resp, err := c.cl.Do(req)

	return resp, err
}

// If the error is not nil, take that.
// If there is an error coming from EOS, return a descriptive error.
func (c *EOSHTTPClient) getRespError(rsp *http.Response, err error) error {
	if err != nil {
		return err
	}

	if rsp.StatusCode == 0 {
		return nil
	}

	switch rsp.StatusCode {
	case 0, http.StatusOK, http.StatusCreated, http.StatusPartialContent:
		return nil
	case http.StatusForbidden:
		return errtypes.PermissionDenied(rspdesc(rsp))
	case http.StatusNotFound:
		return errtypes.NotFound(rspdesc(rsp))
	case http.StatusConflict:
		return errtypes.Conflict(rspdesc(rsp))
	}

	return errtypes.InternalError("Err from EOS: " + rspdesc(rsp))
}

// From the basepath and the file path... build an url.
func (c *EOSHTTPClient) buildFullURL(urlpath string, auth eosclient.Authorization) (string, error) {
	// Prohibit malicious users from injecting a false uid/gid into the url
	pos := strings.Index(urlpath, "?")
	if pos >= 0 {
		if strings.Index(urlpath, "eos.ruid") > pos || strings.Index(urlpath, "eos.rgid") > pos {
			return "", errtypes.PermissionDenied("Illegal malicious url " + urlpath)
		}
	}

	fullurl := strings.TrimRight(c.opt.BaseURL, "/")
	fullurl += "/"
	fullurl += strings.TrimLeft(urlpath, "/")

	if pos < 0 {
		fullurl += "?"
	}

	if auth.Token != "" {
		fullurl += "authz=" + auth.Token
	}

	u, err := url.Parse(fullurl)
	if err != nil {
		return "", errtypes.PermissionDenied("Could not parse url " + urlpath)
	}

	final := strings.ReplaceAll(u.String(), "#", "%23")
	return final, nil
}

// GETFile does an entire GET to download a full file. Returns a stream to read the content from.
func (c *EOSHTTPClient) GETFile(ctx context.Context, remoteuser string, auth eosclient.Authorization, urlpath string, stream io.WriteCloser, ranges []storage.Range) (io.ReadCloser, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "GETFile").Str("remoteuser", remoteuser).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", urlpath).Msg("")

	// Now send the req and see what happens
	finalurl, err := c.buildFullURL(urlpath, auth)
	if err != nil {
		log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return nil, err
	}
	log.Debug().Str("func", "GETFile").Str("url", finalurl).Msg("")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, finalurl, nil)
	if err != nil {
		log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return nil, err
	}
	// similar to eosbinary.go::Read()
	req.Header.Set(eosclient.EosAppHeader, fmt.Sprintf("%s_read", eosclient.EosAppPrefix))
	rangeHeader := ""
	if len(ranges) > 0 {
		var parts []string
		for _, r := range ranges {
			end := r.Start + r.Length - 1
			parts = append(parts, fmt.Sprintf("%d-%d", r.Start, end))
		}
		rangeHeader = "bytes=" + strings.Join(parts, ",")
		req.Header.Set(ocdav.HeaderRange, rangeHeader)
		log.Info().Str("header", rangeHeader).Msg("Setting range header in request to EOS")
	}

	ntries := 0
	nredirs := 0
	timebegin := time.Now().Unix()

	for {
		// Check for a max count of redirections or retries

		// Check for a global timeout in any case
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "GETFile").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return nil, errtypes.InternalError("Timeout with url" + finalurl)
		}

		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream
		log.Debug().Str("func", "GETFile").Str("finalurl", finalurl).Msg("sending req")

		// c.doReq sets headers such as remoteuser and x-gateway-authorization
		// we don't want those when using a token (i.e. ?authz=), so in this case
		// we skip this and call the HTTP client directly
		var resp *http.Response
		if auth.Token != "" {
			resp, err = c.cl.Do(req)
		} else {
			resp, err = c.doReq(req, remoteuser)
		}

		// Let's support redirections... and if we retry we have to retry at the same FST, avoid going back to the MGM
		if resp != nil && (resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect) {
			// io.Copy(ioutil.Discard, resp.Body)
			if resp.Body != nil {
				resp.Body.Close()
			}

			loc, err := resp.Location()
			if err != nil {
				log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't get a new location for a redirection")
				return nil, err
			}

			// Very special case for eos file versions
			final := strings.ReplaceAll(loc.String(), "#", "%23")
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, final, nil)
			if err != nil {
				log.Error().Str("func", "GETFile").Str("url", loc.String()).Str("err", err.Error()).Msg("can't create redirected request")
				return nil, err
			}
			if rangeHeader != "" {
				req.Header.Set(ocdav.HeaderRange, rangeHeader)
			}

			req.Close = true

			log.Debug().Str("func", "GETFile").Str("location", loc.String()).Msg("redirection")
			nredirs++
			resp = nil
			err = nil
			continue
		}

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "GETFile").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}
			log.Error().Str("func", "GETFile").Str("url", finalurl).Str("err", e.Error()).Msg("")
			return nil, e
		}

		log.Debug().Str("func", "GETFile").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return nil, errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}

		if stream != nil {
			// Streaming versus localfile. If we have bene given a dest stream then copy the body into it
			_, err = io.Copy(stream, resp.Body)
			return nil, err
		}

		// If we have not been given a stream to write into then return our stream to read from
		return resp.Body, nil
	}
}

// PUTFile does an entire PUT to upload a full file, taking the data from a stream.
func (c *EOSHTTPClient) PUTFile(ctx context.Context, remoteuser string, auth eosclient.Authorization, urlpath string, stream io.ReadCloser, length int64, app string, disableVersioning bool) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "PUTFile").Str("remoteuser", remoteuser).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", urlpath).Int64("length", length).Str("app", app).Msg("")

	// Now send the req and see what happens
	tempUrl, err := c.buildFullURL(urlpath, auth)
	if err != nil {
		return err
	}
	base, err := url.Parse(tempUrl)
	if err != nil {
		return errtypes.PermissionDenied("Could not parse url " + urlpath)
	}
	queryValues := base.Query()

	if disableVersioning {
		queryValues.Add("eos.versioning", strconv.Itoa(0))
	}
	base.RawQuery = queryValues.Encode()
	finalurl := base.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, finalurl, nil)
	if err != nil {
		log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("app", app).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	// prepare the app tag: if given (e.g. when file is locked), use it, else tag the traffic as write
	if app == "" {
		app = fmt.Sprintf("%s_write", eosclient.EosAppPrefix)
	}
	req.Header.Set(eosclient.EosAppHeader, app)
	req.Close = true

	ntries := 0
	nredirs := 0
	timebegin := time.Now().Unix()

	for {
		// Check for a max count of redirections or retries

		// Check for a global timeout in any case
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "PUTFile").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return errtypes.InternalError("Timeout with url" + finalurl)
		}

		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream
		log.Debug().Str("func", "PUTFile").Any("headers", req.Header).Msg("sending req")

		// c.doReq sets headers such as remoteuser and x-gateway-authorization
		// we don't want those when using a token (i.e. ?authz=), so in this case
		// we skip this and call the HTTP client directly
		var resp *http.Response
		if auth.Token != "" {
			resp, err = c.cl.Do(req)
		} else {
			resp, err = c.doReq(req, remoteuser)
		}

		// Let's support redirections... and if we retry we retry at the same FST
		if resp != nil && resp.StatusCode == 307 {
			// io.Copy(ioutil.Discard, resp.Body)

			loc, err := resp.Location()
			if err != nil {
				log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", err.Error()).Msg("can't get a new location for a redirection")
				return err
			}

			req, err = http.NewRequestWithContext(ctx, http.MethodPut, loc.String(), stream)
			if err != nil {
				log.Error().Str("func", "PUTFile").Str("url", loc.String()).Str("err", err.Error()).Msg("can't create redirected request")
				return err
			}
			if length >= 0 {
				log.Debug().Str("func", "PUTFile").Int64("Content-Length", length).Msg("setting header")
				req.ContentLength = length
				req.Header.Set("Content-Length", fmt.Sprintf("%d", length))
			}

			log.Debug().Str("func", "PUTFile").Str("location", loc.String()).Msg("redirection")
			nredirs++
			if resp.Body != nil {
				resp.Body.Close()
			}
			resp = nil
			err = nil
			continue
		}

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if (resp != nil) && (resp.Body != nil) {
				resp.Body.Close()
			}
			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "PUTFile").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}

			log.Error().Str("func", "PUTFile").Str("url", finalurl).Str("err", e.Error()).Msg("")

			return e
		}

		log.Debug().Str("func", "PUTFile").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}

		if resp.Body != nil {
			resp.Body.Close()
		}
		return nil
	}
}

// Head performs a HEAD req. Useful to check the server.
func (c *EOSHTTPClient) Head(ctx context.Context, remoteuser string, auth eosclient.Authorization, urlpath string) error {
	log := appctx.GetLogger(ctx)
	log.Info().Str("func", "Head").Str("remoteuser", remoteuser).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("path", urlpath).Msg("")

	// Now send the req and see what happens
	finalurl, err := c.buildFullURL(urlpath, auth)
	if err != nil {
		log.Error().Str("func", "Head").Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, finalurl, nil)
	if err != nil {
		log.Error().Str("func", "Head").Str("remoteuser", remoteuser).Str("uid,gid", auth.Role.UID+","+auth.Role.GID).Str("url", finalurl).Str("err", err.Error()).Msg("can't create request")
		return err
	}

	ntries := 0

	timebegin := time.Now().Unix()
	for {
		tdiff := time.Now().Unix() - timebegin
		if tdiff > int64(c.opt.OpTimeout) {
			log.Error().Str("func", "Head").Str("url", finalurl).Int64("timeout", tdiff).Int("ntries", ntries).Msg("")
			return errtypes.InternalError("Timeout with url" + finalurl)
		}
		// Execute the request. I don't like that there is no explicit timeout or buffer control on the input stream

		resp, err := c.doReq(req, remoteuser)

		// And get an error code (if error) that is worth propagating
		e := c.getRespError(resp, err)
		if e != nil {
			if os.IsTimeout(e) {
				ntries++
				log.Warn().Str("func", "Head").Str("url", finalurl).Str("err", e.Error()).Int("try", ntries).Msg("recoverable network timeout")
				continue
			}
			log.Error().Str("func", "Head").Str("url", finalurl).Str("err", e.Error()).Msg("")
			return e
		}
		if resp != nil {
			defer resp.Body.Close()
		}

		log.Debug().Str("func", "Head").Str("url", finalurl).Str("resp:", fmt.Sprintf("%#v", resp)).Msg("")
		if resp == nil {
			return errtypes.NotFound(fmt.Sprintf("url: %s", finalurl))
		}
	}
	// return nil
}
