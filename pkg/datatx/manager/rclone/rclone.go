// Copyright 2018-2020 CERN
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

package rclone

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	registry "github.com/cs3org/reva/pkg/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("rclone", New)
}

func (c *config) init(m map[string]interface{}) {
	// set sane defaults
}

type config struct {
	Endpoint string `mapstructure:"endpoint"`
	AuthUser string `mapstructure:"auth_user"`
	AuthPass string `mapstructure:"auth_pass"`
}

type rclone struct {
	config *config
	client *http.Client
}

// New returns a new rclone driver
func New(m map[string]interface{}) (txdriver.TxDriver, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	// TODO insecure should be configurable
	client := rhttp.GetHTTPClient(rhttp.Insecure(true))

	return &rclone{
		config: c,
		client: client,
	}, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// DoTransfer initiates a transfer and returns the transfer job id.
// If jobID -1 is returned it means that the transfer could not be initiated; and an error is possibly returned with it.
func (driver *rclone) DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (int64, error) {
	// In order to be able to work with the current rclone setup we have to work with a remote for each user.
	// For that we concatenate the username to the specified remote. And under that label the remotes
	// are specified in the rclone config.
	// The remote is in fact the idp + username transformed somewhat to: '{ipd}-{username}:'
	// 		eg. idp 'surfsara.nl' for user 'marie' becomes:
	//			'surfsara_nl-marie:'
	//
	//		The specification in the rclone config should then look like:
	//
	//		[surfsara_nl-marie]
	//		type = webdav
	//		url = https://app.cs3mesh-iop.k8s.surfsara.nl/iop/
	//		user = antoon
	//		pass = P9MxNDEPhYXaxjjO2sTTH0A3jGQnI6SWsd8
	//		vendor = owncloud
	//
	//	And that way rclone can find src and dest remotes' configs using the specified srcRemote, srcToken and destRemote
	//
	// TODO: re-implement when the connection-string-remote syntax is available in rclone.

	// replace '.' with '_'; concatenate -{token}; concatenate ':'
	reqSrcRemote := strings.ReplaceAll(srcRemote, ".", "_") + "-" + srcToken + ":"
	reqDestRemote := strings.ReplaceAll(destRemote, ".", "_") + "-" + destToken + ":"

	type rcloneAsyncReqJSON struct {
		SrcRemote string `json:"srcFs"`
		SrcPath   string `json:"srcRemote"`
		// SrcToken string `json:"srcToken"`
		DstRemote string `json:"dstFs"`
		DstPath   string `json:"dstRemote"`
		// DstToken string `json:"srcToken"`
		Async bool `json:"_async"`
	}
	rcloneReq := &rcloneAsyncReqJSON{
		SrcRemote: reqSrcRemote,
		SrcPath:   srcPath,
		DstRemote: reqDestRemote,
		DstPath:   destPath,
		Async:     true,
	}

	data, err := json.Marshal(rcloneReq)
	if err != nil {
		return -1, errors.Wrap(err, "error marshalling rclone req data")
	}

	// TODO handle directory transfers
	transferFileMethod := "/operations/copyfile"

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err = errors.Wrap(err, "json: error parsing driver endpoint")
		return -1, err
	}
	u.Path = path.Join(u.Path, transferFileMethod)
	requestURL := u.String()
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(data))
	if err != nil {
		return -1, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

	res, err := driver.client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return -1, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			err = errors.Wrap(err, "json: error reading response body")
			return -1, err
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return -1, err
	}

	type rcloneAsyncResJSON struct {
		JobID int64 `json:"jobid"`
	}
	var resData rcloneAsyncResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		err = errors.Wrap(err, "json: error decoding response data")
		return -1, err
	}

	return resData.JobID, nil
}

// GetTransferStatus returns the status of the transfer with the specified job id
func (driver *rclone) GetTransferStatus(jobID int64) (datatx.TxInfo_Status, error) {
	type rcloneStatusReqJSON struct {
		JobID int64 `json:"jobid"`
	}
	rcloneStatusReq := &rcloneStatusReqJSON{
		JobID: jobID,
	}

	data, err := json.Marshal(rcloneStatusReq)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "error marshalling rclone req data")
	}

	transferFileMethod := "/job/status"

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err = errors.Wrap(err, "json: error parsing driver endpoint")
		return -1, err
	}
	u.Path = path.Join(u.Path, transferFileMethod)
	requestURL := u.String()

	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(data))
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

	res, err := driver.client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TODO "job not found" also gives a 500
		// Should that return STATUS_INVALID ??
		// at the minimum the returned error message should be the rclone error message
		resBody, e := ioutil.ReadAll(res.Body)
		if e != nil {
			e = errors.Wrap(e, "json: error reading response body")
			return datatx.TxInfo_STATUS_INVALID, e
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	type rcloneStatusResJSON struct {
		Finished  bool    `json:"finished"`
		Success   bool    `json:"success"`
		ID        int64   `json:"id"`
		Error     string  `json:"error"`
		Group     string  `json:"group"`
		StartTime string  `json:"startTime"`
		EndTime   string  `json:"endTime"`
		Duration  float64 `json:"duration"`
		// think we don't need this
		// "output": {} // output of the job as would have been returned if called synchronously
	}
	var resData rcloneStatusResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "json: error decoding response data")
	}

	if resData.Error != "" {
		return datatx.TxInfo_STATUS_INVALID, errors.New(resData.Error)
	}

	if resData.Finished && resData.Success {
		return datatx.TxInfo_STATUS_TRANSFER_COMPLETE, nil
	}
	if !resData.Finished {
		return datatx.TxInfo_STATUS_TRANSFER_IN_PROGRESS, nil
	}
	if !resData.Success {
		return datatx.TxInfo_STATUS_TRANSFER_FAILED, nil
	}
	return datatx.TxInfo_STATUS_INVALID, nil
}

// CancelTransfer cancels the transfer with the specified job id
func (driver *rclone) CancelTransfer(jobID int64) (datatx.TxInfo_Status, error) {
	// rcloneCancelTransferReqJSON the rclone job/stop method json request
	type rcloneCancelTransferReqJSON struct {
		JobID int64 `json:"jobid"`
	}
	rcloneCancelTransferReq := &rcloneCancelTransferReqJSON{
		JobID: jobID,
	}

	data, err := json.Marshal(rcloneCancelTransferReq)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "error marshalling rclone req data")
	}

	transferFileMethod := "/job/stop"

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err = errors.Wrap(err, "json: error parsing driver endpoint")
		return -1, err
	}
	u.Path = path.Join(u.Path, transferFileMethod)
	requestURL := u.String()

	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(data))
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

	res, err := driver.client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TODO "job not found" also gives a 500
		// Should that return STATUS_INVALID ??
		// at the minimum the returned error message should be the rclone error message
		resBody, e := ioutil.ReadAll(res.Body)
		if e != nil {
			e = errors.Wrap(e, "json: error reading response body")
			return datatx.TxInfo_STATUS_INVALID, e
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	type rcloneCancelTransferResJSON struct {
		Finished  bool    `json:"finished"`
		Success   bool    `json:"success"`
		ID        int64   `json:"id"`
		Error     string  `json:"error"`
		Group     string  `json:"group"`
		StartTime string  `json:"startTime"`
		EndTime   string  `json:"endTime"`
		Duration  float64 `json:"duration"`
		// think we don't need this
		// "output": {} // output of the job as would have been returned if called synchronously
	}
	var resData rcloneCancelTransferResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "json: error decoding response data")
	}

	if resData.Error != "" {
		return datatx.TxInfo_STATUS_TRANSFER_CANCEL_FAILED, errors.New(resData.Error)
	}
	// an empty response means success
	return datatx.TxInfo_STATUS_TRANSFER_CANCELLED, nil
}
