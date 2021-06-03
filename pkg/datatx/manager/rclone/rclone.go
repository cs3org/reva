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
	"os"
	"path"
	"sync"
	"time"

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
	if c.File == "" {
		c.File = "/var/tmp/reva/datatx-transfers.json"
	}
}

type config struct {
	Endpoint               string `mapstructure:"endpoint"`
	AuthUser               string `mapstructure:"auth_user"` // rclone basicauth user
	AuthPass               string `mapstructure:"auth_pass"` // rclone basicauth pass
	File                   string `mapstructure:"file"`
	JobStatusCheckInterval int    `mapstructure:"job_status_check_interval"`
	JobTimeout             int    `mapstructure:"job_timeout"`
}

type rclone struct {
	config  *config
	client  *http.Client
	pDriver *pDriver
}

type transferModel struct {
	File      string
	Transfers map[string]*transfer `json:"transfers"`
}

// persistency driver
type pDriver struct {
	sync.Mutex // concurrent access to the file
	model      *transferModel
}

type transfer struct {
	TransferID     string
	JobID          int64
	TransferStatus datatx.TxInfo_Status
	SrcRemote      string
	SrcPath        string
	DestRemote     string
	DestPath       string
}

// txEndStatuses final statuses that cannot be changed anymore
var txEndStatuses = map[string]int32{
	"STATUS_INVALID":                0,
	"STATUS_DESTINATION_NOT_FOUND":  1,
	"STATUS_TRANSFER_COMPLETE":      6,
	"STATUS_TRANSFER_FAILED":        7,
	"STATUS_TRANSFER_CANCELLED":     8,
	"STATUS_TRANSFER_CANCEL_FAILED": 9,
	"STATUS_TRANSFER_EXPIRED":       10,
}

// New returns a new rclone driver
func New(m map[string]interface{}) (txdriver.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	// TODO insecure should be configurable
	client := rhttp.GetHTTPClient(rhttp.Insecure(true))

	// The persistency driver
	// Load or create 'db'
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the transfers")
		return nil, err
	}
	pDriver := &pDriver{
		model: model,
	}

	return &rclone{
		config:  c,
		client:  client,
		pDriver: pDriver,
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

func loadOrCreate(file string) (*transferModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "error creating the transfers storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the transfers storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	model := &transferModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "error decoding transfers data to json")
		return nil, err
	}

	if model.Transfers == nil {
		model.Transfers = make(map[string]*transfer)
	}

	model.File = file
	return model, nil
}

func (m *transferModel) SaveTransfer() error {
	data, err := json.Marshal(m)
	if err != nil {
		err = errors.Wrap(err, "error encoding transfer data to json")
		return err
	}

	if err := ioutil.WriteFile(m.File, data, 0644); err != nil {
		err = errors.Wrap(err, "error writing transfer data to file: "+m.File)
		return err
	}

	return nil
}

// DoTransfer initiates a transfer and returns the transfer job id.
// If jobID -1 is returned it means that the transfer could not be initiated; and an error is possibly returned with it.
func (driver *rclone) CreateTransfer(transferID string, srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (datatx.TxInfo_Status, error) {
	// TODO ctx param for eg. logging ?
	fmt.Printf("rclone CreateTransfer:\n   transferID: %v \n   srcRemote: %v \n   srcPath: %v \n   srcToken: %v \n   destRemote: %v \n   destPath: %v \n   destToken: %v \n", transferID, srcRemote, srcPath, srcToken, destRemote, destPath, destToken)

	type rcloneAsyncReqJSON struct {
		SrcFs    string `json:"srcFs"`
		SrcToken string `json:"srcToken"`
		DstFs    string `json:"dstFs"`
		DstToken string `json:"destToken"`
		Async    bool   `json:"_async"`
	}
	// TODO remove schema from url (part of idp? or configurable?)
	srcFs := fmt.Sprintf(":webdav,headers=\"x-access-token,%v\",url=\"http://%v/webdav\":%v", srcToken, srcRemote, srcPath)
	dstFs := fmt.Sprintf(":webdav,headers=\"x-access-token,%v\",url=\"http://%v/webdav\":%v", destToken, destRemote, destPath)
	rcloneReq := &rcloneAsyncReqJSON{
		SrcFs: srcFs,
		DstFs: dstFs,
		Async: true,
	}
	data, err := json.Marshal(rcloneReq)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "error marshalling rclone req data")
	}

	// TODO test if path is folder or file (stat src?)
	pathIsFolder := true
	transferFileMethod := "/operations/copyfile"
	if pathIsFolder {
		// TODO sync/copy will overwrite existing data; use a configurable check for this?
		// But not necessary if unique folder per transfer
		transferFileMethod = "/sync/copy"
	}

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err = errors.Wrap(err, "json: error parsing driver endpoint")
		return datatx.TxInfo_STATUS_INVALID, err
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
		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			err = errors.Wrap(err, "json: error reading response body")
			return datatx.TxInfo_STATUS_INVALID, err
		}
		err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", res.Status, string(resBody))), "json: rclone request responded with error")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	type rcloneAsyncResJSON struct {
		JobID int64 `json:"jobid"`
	}
	var resData rcloneAsyncResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		err = errors.Wrap(err, "json: error decoding response data")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	transferStatus := datatx.TxInfo_STATUS_TRANSFER_NEW
	transfer := &transfer{
		TransferID:     transferID,
		JobID:          resData.JobID,
		TransferStatus: transferStatus,
		SrcRemote:      srcRemote,
		SrcPath:        srcPath,
		DestRemote:     destRemote,
		DestPath:       destPath,
	}

	driver.pDriver.Lock()
	defer driver.pDriver.Unlock()

	driver.pDriver.model.Transfers[transferID] = transfer

	if err := driver.pDriver.model.SaveTransfer(); err != nil {
		err = errors.Wrap(err, "json: error saving transfer")
		return datatx.TxInfo_STATUS_INVALID, err
	}

	// start separate dedicated process to periodically check this transfer progress
	go func() {
		// runs for as long as no end state or time out has been reached
		startTimeMs := time.Now().Nanosecond() / 1000
		timeout := driver.config.JobTimeout

		driver.pDriver.Lock()
		defer driver.pDriver.Unlock()

		for {
			transfer, err := driver.pDriver.model.GetTransfer(transferID)
			if err != nil {
				fmt.Printf("error: %v\n", err)
				// TODO log ?
				break
			}
			// check for end status first
			endStatus, endStatusFound := txEndStatuses[transfer.TransferStatus.String()]
			if endStatusFound {
				fmt.Printf("end status reached: %v\n", endStatus)
				break
			}

			// check for possible timeout and if true were done
			currentTimeMs := time.Now().Nanosecond() / 1000
			timePastMs := currentTimeMs - startTimeMs

			if timePastMs > timeout {
				fmt.Printf("Transfer timed out: %vms (timeout = %v)\n", timePastMs, timeout)
				// set status to EXPIRED and save
				transfer.TransferStatus = datatx.TxInfo_STATUS_TRANSFER_EXPIRED
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					// log this?
					fmt.Printf("Save transfer failed: %v \n", err)
				}
				break
			}

			// request rclone for current job status
			//
			// TODO: what do we do in case calling rclone results in errors ?
			// simply break, or log|save(status invalid)|break ?
			// or don't break with any error and try until successful or expiration ?
			// for now break on any error

			jobID := transfer.JobID

			type rcloneStatusReqJSON struct {
				JobID int64 `json:"jobid"`
			}
			rcloneStatusReq := &rcloneStatusReqJSON{
				JobID: jobID,
			}

			data, err := json.Marshal(rcloneStatusReq)
			if err != nil {
				fmt.Printf("err: marshalling request failed: %v\n", err)
				// save the transfer with status invalid
				transfer.TransferStatus = datatx.TxInfo_STATUS_INVALID
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					// log this?
					break
				}
				break
			}

			transferFileMethod := "/job/status"

			u, err := url.Parse(driver.config.Endpoint)
			if err != nil {
				fmt.Printf("err: could not parse driver endpoint: %v\n", err)
				// log this ? "json: error parsing driver endpoint"
				break
			}
			u.Path = path.Join(u.Path, transferFileMethod)
			requestURL := u.String()

			req, err := http.NewRequest("POST", requestURL, bytes.NewReader(data))
			if err != nil {
				fmt.Printf("err: error framing post request: %v\n", err)
				// log this ? "json: error framing post request"
				break
			}
			req.Header.Set("Content-Type", "application/json")

			req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

			res, err := driver.client.Do(req)
			if err != nil {
				fmt.Printf("err: error sending post request: %v\n", err)
				// log this ? "json: error sending post request"
				break
			}

			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				// TODO "job not found" also gives a 500
				// Should that return STATUS_INVALID ??
				// at the minimum the returned error message should be the rclone error message
				resBody, e := ioutil.ReadAll(res.Body)
				if e != nil {
					fmt.Printf("err: error reading response body: %v \n", err)
				}
				fmt.Printf("json: rclone request responded with error: %s: %s \n", res.Status, string(resBody))
				break
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
				fmt.Printf("err: error decoding response data: %v\n", err)
				// log this ? "json: error decoding response data"
				break
			}

			fmt.Printf("rclone resData: %v\n", resData)

			if resData.Error != "" {
				// log this ? resData.Error
				fmt.Printf("err(rclone): %v\n", resData.Error)
				transfer.TransferStatus = datatx.TxInfo_STATUS_TRANSFER_FAILED
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					fmt.Printf("err: error saving transfer: %v\n", err)
					// log this?
					break
				}
				break
			}

			// transfer complete
			if resData.Finished && resData.Success {
				fmt.Println("transfer job finished")
				transfer.TransferStatus = datatx.TxInfo_STATUS_TRANSFER_COMPLETE
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					fmt.Printf("err: error saving transfer: %v\n", err)
					// log this?
					break
				}
				break
			}

			// transfer completed unsuccessfully without error
			if resData.Finished && !resData.Success {
				fmt.Println("transfer job failed")
				transfer.TransferStatus = datatx.TxInfo_STATUS_TRANSFER_FAILED
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					fmt.Printf("err: error saving transfer: %v\n", err)
					// log this?
					break
				}
				break
			}

			// transfer not yet finished: continue
			if !resData.Finished {
				fmt.Println("transfer job in progress")
				transfer.TransferStatus = datatx.TxInfo_STATUS_TRANSFER_IN_PROGRESS
				if err := driver.pDriver.model.SaveTransfer(); err != nil {
					fmt.Printf("err: error saving transfer: %v\n", err)
					// log this?
					break
				}
			}

			fmt.Printf("... checking\n")
			<-time.After(time.Millisecond * time.Duration(driver.config.JobStatusCheckInterval))
		}
	}()

	return transferStatus, nil
}

// GetTransferStatus returns the status of the transfer with the specified job id
func (driver *rclone) GetTransferStatus(transferID string) (datatx.TxInfo_Status, error) {
	// does transfer exist?
	transfer, err := driver.pDriver.model.GetTransfer(transferID)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, err
	}
	return transfer.TransferStatus, nil
}

// GetTransfer returns the transfer with the specified transfer ID
func (m *transferModel) GetTransfer(transferID string) (*transfer, error) {
	transfer, ok := m.Transfers[transferID]
	if !ok {
		return nil, errors.New("json: invalid transfer ID")
	}
	return transfer, nil
}

// CancelTransfer cancels the transfer with the specified transfer id
func (driver *rclone) CancelTransfer(transferID string) (datatx.TxInfo_Status, error) {
	transfer, err := driver.pDriver.model.GetTransfer(transferID)
	if err != nil {
		return datatx.TxInfo_STATUS_INVALID, err
	}
	_, endStatusFound := txEndStatuses[transfer.TransferStatus.String()]
	if endStatusFound {
		return datatx.TxInfo_STATUS_INVALID, errors.Wrap(err, "transfer already in end state")
	}

	// rcloneCancelTransferReqJSON the rclone job/stop method json request
	type rcloneCancelTransferReqJSON struct {
		JobID int64 `json:"jobid"`
	}
	rcloneCancelTransferReq := &rcloneCancelTransferReqJSON{
		JobID: transfer.JobID,
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
	// an empty response means cancelation went successfully
	return datatx.TxInfo_STATUS_TRANSFER_CANCELLED, nil
}
