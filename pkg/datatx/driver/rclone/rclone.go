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
	"strings"

	txdriver "github.com/cs3org/reva/pkg/datatx/driver"
	config "github.com/cs3org/reva/pkg/datatx/driver/config"
	registry "github.com/cs3org/reva/pkg/datatx/driver/registry"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/pkg/errors"
)

func init() {
	driver := &Rclone{}
	registry.Register(driverName(), driver)
}

// Rclone the rclone driver
type Rclone struct {
	SenderEndpoint string
}

func driverName() string {
	return "rclone"
}

// Configure configures this driver
func (driver *Rclone) Configure(c *config.Config) error {
	if c.DataTxSenderEndpoint == "" {
		err := errors.New("Unable to initialize a data transfer driver, has the transfer sender endpoint been specified?")
		return err
	}

	driver.SenderEndpoint = c.DataTxSenderEndpoint

	return nil
}

// DoTransfer initiates a transfer and returns the transfer job ID
// If Job.jobID -1 is returned it means that the transfer could not be initiated
func (driver *Rclone) DoTransfer(srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (*txdriver.Job, error) {
	// example call from surfsara to cernbox
	//  - the users are to be defined with the remotes in the rclone config
	//  - basic http auth is used (-u user:pass)
	//
	// The example call:
	// curl
	// 	-u user:pass
	// 	-H "Content-Type: application/json"
	// 	-X POST
	// 	-d '{"srcFs":"surfsara:", "srcRemote":"/webdav/home/message-from-surfsara.txt", "dstFs":"cernbox:", "dstRemote":"/webdav/home/tmp/message-from-surfsara.txt", "_async":true}'
	// 	http://localhost:5572/operations/copyfile
	//
	//
	// 1. prepare config: add src/dest remotes

	// 2. request for an async rclone call
	// convert remote names to rclone type fs ('remotename:')
	rcloneReq := &rcloneAsyncReqJSON{
		SrcRemote: strings.Trim(srcRemote, ":") + ":",
		SrcPath:   srcPath,
		DstRemote: strings.Trim(destRemote, ":") + ":",
		DstPath:   destPath,
		Async:     true,
	}

	data, err := json.Marshal(rcloneReq)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling rclone req data")
	}

	// TODO handle directory transfers
	transferFileMethod := "/operations/copyfile"
	url := strings.Trim(driver.SenderEndpoint, "/") + transferFileMethod
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	// TODO if we want to use basic auth, should be configurable.
	req.SetBasicAuth("rclone", "secret")

	// TODO insecure should be configurable
	client := rhttp.GetHTTPClient(rhttp.Insecure(true))
	res, err := client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			err = errors.Wrap(err, "json: error reading response body")
			return nil, err
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return nil, err
	}

	var resData rcloneAsyncResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		err = errors.Wrap(err, "json: error decoding response data")
		return nil, err
	}

	// create new Job from jobID
	newJob := txdriver.Job{
		JobID:      resData.JobID,
		SrcRemote:  txdriver.Remote{OpaqueName: srcRemote},
		DestRemote: txdriver.Remote{OpaqueName: destRemote},
	}
	return &newJob, nil
}

// rcloneStatusReqJSON the rclone job/status method json request
type rcloneStatusReqJSON struct {
	JobID int64 `json:"jobid"`
}

// rcloneStatusResJSON the rclone job/status method json response
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

// GetTransferStatus returns the status of the transfer defined by the specified job
func (driver *Rclone) GetTransferStatus(job *txdriver.Job) (string, error) {
	rcloneStatusReq := &rcloneStatusReqJSON{
		JobID: job.JobID,
	}

	data, err := json.Marshal(rcloneStatusReq)
	if err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "error marshalling rclone req data")
	}

	transferFileMethod := "/job/status"
	url := strings.Trim(driver.SenderEndpoint, "/") + transferFileMethod
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	// TODO if we want to use basic auth, should be configurable.
	req.SetBasicAuth("rclone", "secret")

	// TODO insecure should be configurable
	client := rhttp.GetHTTPClient(rhttp.Insecure(true))
	res, err := client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return txdriver.StatusInvalid, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TODO "job not found" also gives a 500
		// Should that return STATUS_INVALID ??
		// at the minimum the returned error message should be the rclone error message
		resBody, e := ioutil.ReadAll(res.Body)
		if e != nil {
			e = errors.Wrap(e, "json: error reading response body")
			return txdriver.StatusInvalid, e
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return txdriver.StatusInvalid, err
	}

	var resData rcloneStatusResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "json: error decoding response data")
	}

	if resData.Error != "" {
		return txdriver.StatusInvalid, errors.New(resData.Error)
	}

	if resData.Finished && resData.Success {
		return txdriver.StatusTransferComplete, nil
	}
	if !resData.Finished {
		return txdriver.StatusTransferInProgress, nil
	}
	if !resData.Success {
		return txdriver.StatusTransferFailed, nil
	}
	return txdriver.StatusInvalid, nil
}

// rcloneCancelTransferReqJSON the rclone job/stop method json request
type rcloneCancelTransferReqJSON struct {
	JobID int64 `json:"jobid"`
}

// rcloneCancelTransferResJSON the rclone job/stop method json response
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

// CancelTransfer cancels the transfer defined by the specified job
func (driver *Rclone) CancelTransfer(job *txdriver.Job) (string, error) {
	rcloneCancelTransferReq := &rcloneCancelTransferReqJSON{
		JobID: job.JobID,
	}

	data, err := json.Marshal(rcloneCancelTransferReq)
	if err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "error marshalling rclone req data")
	}

	transferFileMethod := "/job/stop"
	url := strings.Trim(driver.SenderEndpoint, "/") + transferFileMethod
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "json: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	// TODO if we want to use basic auth, should be configurable.
	req.SetBasicAuth("rclone", "secret")

	// TODO insecure should be configurable
	client := rhttp.GetHTTPClient(rhttp.Insecure(true))
	res, err := client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return txdriver.StatusInvalid, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// TODO "job not found" also gives a 500
		// Should that return STATUS_INVALID ??
		// at the minimum the returned error message should be the rclone error message
		resBody, e := ioutil.ReadAll(res.Body)
		if e != nil {
			e = errors.Wrap(e, "json: error reading response body")
			return txdriver.StatusInvalid, e
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return txdriver.StatusInvalid, err
	}

	var resData rcloneCancelTransferResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		return txdriver.StatusInvalid, errors.Wrap(err, "json: error decoding response data")
	}

	if resData.Error != "" {
		return txdriver.StatusTransferCancelFailed, errors.New(resData.Error)
	}
	// an empty response means success
	return txdriver.StatusTransferCancelled, nil
}

// rcloneAsyncReqJSON the rclone operations/filecopy async method json request
type rcloneAsyncReqJSON struct {
	SrcRemote string `json:"srcFs"`
	SrcPath   string `json:"srcRemote"`
	// SrcToken string `json:"srcToken"`
	DstRemote string `json:"dstFs"`
	DstPath   string `json:"dstRemote"`
	// DstToken string `json:"srcToken"`
	Async bool `json:"_async"`
}

// rcloneAsyncResJSON the rclone operations/copyfile async method json response
type rcloneAsyncResJSON struct {
	JobID int64 `json:"jobid"`
}
