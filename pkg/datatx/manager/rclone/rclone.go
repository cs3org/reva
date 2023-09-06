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

package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	txdriver "github.com/cs3org/reva/pkg/datatx"
	"github.com/cs3org/reva/pkg/datatx/manager/rclone/repository"
	repoRegistry "github.com/cs3org/reva/pkg/datatx/manager/rclone/repository/registry"
	registry "github.com/cs3org/reva/pkg/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("rclone", New)
}

func (c *config) init(m map[string]interface{}) {
	// set sane defaults
	if c.JobStatusCheckInterval == 0 {
		c.JobStatusCheckInterval = 2000
	}
	if c.JobTimeout == 0 {
		c.JobTimeout = 50000
	}
}

type config struct {
	Endpoint                  string                            `mapstructure:"endpoint"`
	AuthUser                  string                            `mapstructure:"auth_user"` // rclone basicauth user
	AuthPass                  string                            `mapstructure:"auth_pass"` // rclone basicauth pass
	AuthHeader                string                            `mapstructure:"auth_header"`
	JobStatusCheckInterval    int                               `mapstructure:"job_status_check_interval"`
	JobTimeout                int                               `mapstructure:"job_timeout"`
	Insecure                  bool                              `mapstructure:"insecure"`
	RemoveTransferJobOnCancel bool                              `mapstructure:"remove_transfer_job_on_cancel"`
	StorageDriver             string                            `mapstructure:"storagedriver"`
	StorageDrivers            map[string]map[string]interface{} `mapstructure:"storagedrivers"`
}

type rclone struct {
	config  *config
	client  *http.Client
	storage repository.Repository
}

type rcloneHTTPErrorRes struct {
	Error  string                 `json:"error"`
	Input  map[string]interface{} `json:"input"`
	Path   string                 `json:"path"`
	Status int                    `json:"status"`
}

// txEndStatuses final statuses that cannot be changed anymore.
var txEndStatuses = map[string]int32{
	"STATUS_INVALID":                0,
	"STATUS_DESTINATION_NOT_FOUND":  1,
	"STATUS_TRANSFER_COMPLETE":      6,
	"STATUS_TRANSFER_FAILED":        7,
	"STATUS_TRANSFER_CANCELLED":     8,
	"STATUS_TRANSFER_CANCEL_FAILED": 9,
	"STATUS_TRANSFER_EXPIRED":       10,
}

type endpoint struct {
	filePath       string
	endpoint       string
	endpointScheme string
	token          string
}

// New returns a new rclone driver.
func New(ctx context.Context, m map[string]interface{}) (txdriver.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	client := rhttp.GetHTTPClient(rhttp.Insecure(c.Insecure))

	storage, err := getStorageManager(ctx, c)
	if err != nil {
		return nil, err
	}

	return &rclone{
		config:  c,
		client:  client,
		storage: storage,
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

func getStorageManager(ctx context.Context, c *config) (repository.Repository, error) {
	if f, ok := repoRegistry.NewFuncs[c.StorageDriver]; ok {
		return f(ctx, c.StorageDrivers[c.StorageDriver])
	}
	return nil, errtypes.NotFound("rclone service: storage driver not found: " + c.StorageDriver)
}

// CreateTransfer creates a transfer job and returns a TxInfo object that includes a unique transfer id.
// Specified target URIs are of form scheme://userinfo@host:port?name={path}
func (driver *rclone) CreateTransfer(ctx context.Context, srcTargetURI string, dstTargetURI string) (*datatx.TxInfo, error) {
	log := appctx.GetLogger(ctx)
	srcEp, err := driver.extractEndpointInfo(ctx, srcTargetURI)
	if err != nil {
		return nil, err
	}
	srcRemote := fmt.Sprintf("%s://%s", srcEp.endpointScheme, srcEp.endpoint)
	srcPath := srcEp.filePath
	srcToken := srcEp.token

	destEp, err := driver.extractEndpointInfo(ctx, dstTargetURI)
	if err != nil {
		return nil, err
	}
	dstPath := destEp.filePath
	dstToken := destEp.token
	// we always set the userinfo part of the destination url for rclone tpc push support
	dstRemote := fmt.Sprintf("%s://%s@%s", destEp.endpointScheme, dstToken, destEp.endpoint)

	log.Debug().Str("srcRemote", srcRemote).Str("srcPath", srcPath).Str("srcToken", srcToken).Str("dstRemote", dstRemote).Str("dstPath", dstPath).Str("dstToken", dstToken).Msg("starting rclone job")
	return driver.startJob(ctx, "", srcRemote, srcPath, srcToken, dstRemote, dstPath, dstToken)
}

// startJob starts a transfer job. Retries a previous job if transferID is specified.
func (driver *rclone) startJob(ctx context.Context, transferID string, srcRemote string, srcPath string, srcToken string, destRemote string, destPath string, destToken string) (*datatx.TxInfo, error) {
	logger := appctx.GetLogger(ctx)

	var txID string
	var cTime *typespb.Timestamp

	if transferID == "" {
		txID = uuid.New().String()
		cTime = &typespb.Timestamp{Seconds: uint64(time.Now().Unix())}
	} else { // restart existing transfer job if transferID is specified
		logger.Debug().Msgf("Restarting transfer job (txID: %s)", transferID)
		txID = transferID
		job, err := driver.storage.GetJob(txID)
		if err != nil {
			err = errors.Wrap(err, "rclone: error retrying transfer job (transferID:  "+txID+")")
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: txID},
				Status: datatx.Status_STATUS_INVALID,
				Ctime:  nil,
			}, err
		}
		seconds, _ := strconv.ParseInt(job.Ctime, 10, 64)
		cTime = &typespb.Timestamp{Seconds: uint64(seconds)}
		_, endStatusFound := txEndStatuses[job.TransferStatus.String()]
		if !endStatusFound {
			err := errors.New("rclone: job still running, unable to restart")
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: txID},
				Status: job.TransferStatus,
				Ctime:  cTime,
			}, err
		}
		srcToken = job.SrcToken
		srcRemote = job.SrcRemote
		srcPath = job.SrcPath
		destToken = job.DestToken
		destRemote = job.DestRemote
		destPath = job.DestPath
		if err := driver.storage.DeleteJob(job); err != nil {
			err = errors.Wrap(err, "rclone: transfer still running, unable to restart")
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: txID},
				Status: job.TransferStatus,
				Ctime:  cTime,
			}, err
		}
	}

	transferStatus := datatx.Status_STATUS_TRANSFER_NEW

	job := &repository.Job{
		TransferID:     txID,
		JobID:          int64(-1),
		TransferStatus: transferStatus,
		SrcToken:       srcToken,
		SrcRemote:      srcRemote,
		SrcPath:        srcPath,
		DestToken:      destToken,
		DestRemote:     destRemote,
		DestPath:       destPath,
		Ctime:          fmt.Sprint(cTime.Seconds), // TODO do we need nanos here?
	}

	type rcloneAsyncReqJSON struct {
		SrcFs string `json:"srcFs"`
		DstFs string `json:"dstFs"`
		Async bool   `json:"_async"`
	}
	// bearer is the default authentication scheme for reva
	srcAuthHeader := fmt.Sprintf("bearer_token=\"%v\"", srcToken)
	if driver.config.AuthHeader == "x-access-token" {
		srcAuthHeader = fmt.Sprintf("headers=\"x-access-token,%v\"", srcToken)
	}
	srcFs := fmt.Sprintf(":webdav,%v,url=\"%v\":%v", srcAuthHeader, srcRemote, srcPath)
	destAuthHeader := fmt.Sprintf("bearer_token=\"%v\"", destToken)
	if driver.config.AuthHeader == "x-access-token" {
		destAuthHeader = fmt.Sprintf("headers=\"x-access-token,%v\"", destToken)
	}
	dstFs := fmt.Sprintf(":webdav,%v,url=\"%v\":%v", destAuthHeader, destRemote, destPath)
	rcloneReq := &rcloneAsyncReqJSON{
		SrcFs: srcFs,
		DstFs: dstFs,
		Async: true,
	}
	data, err := json.Marshal(rcloneReq)
	if err != nil {
		err = errors.Wrap(err, "rclone: transfer job error: error marshalling rclone req data")
		job.TransferStatus = datatx.Status_STATUS_INVALID
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	transferFileMethod := "/sync/copy"
	remotePathIsFolder, err := driver.remotePathIsFolder(srcRemote, srcPath, srcToken)
	if err != nil {
		err = errors.Wrap(err, "rclone: transfer job error: error stating src path")
		job.TransferStatus = datatx.Status_STATUS_INVALID
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}
	if !remotePathIsFolder {
		err = errors.Wrap(err, "rclone: transfer job error: path is a file, only folder transfer is implemented")
		job.TransferStatus = datatx.Status_STATUS_INVALID
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err = errors.Wrap(err, "rclone: transfer job error: error parsing driver endpoint")
		job.TransferStatus = datatx.Status_STATUS_INVALID
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}
	u.Path = path.Join(u.Path, transferFileMethod)
	requestURL := u.String()
	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		err = errors.Wrap(err, "rclone: transfer job error: error framing post request")
		job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)
	res, err := driver.client.Do(req)
	if err != nil {
		err = errors.Wrap(err, "rclone: transfer job error: error sending post request")
		job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorResData rcloneHTTPErrorRes
		if err = json.NewDecoder(res.Body).Decode(&errorResData); err != nil {
			err = errors.Wrap(err, "rclone driver: error decoding rclone response data")
			job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
			var e error
			if e = driver.storage.StoreJob(job); e != nil {
				e = errors.Wrap(e, err.Error())
			}
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: txID},
				Status: job.TransferStatus,
				Ctime:  cTime,
			}, e
		}
		err := errors.New("rclone driver: rclone request responded with error, " + fmt.Sprintf(" status: %v, error: %v", errorResData.Status, errorResData.Error))
		job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	type rcloneAsyncResJSON struct {
		JobID int64 `json:"jobid"`
	}
	var resData rcloneAsyncResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		err = errors.Wrap(err, "rclone driver: error decoding response data")
		job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
		var e error
		if e = driver.storage.StoreJob(job); e != nil {
			e = errors.Wrap(e, err.Error())
		}
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: job.TransferStatus,
			Ctime:  cTime,
		}, e
	}

	job.JobID = resData.JobID

	if err := driver.storage.StoreJob(job); err != nil {
		err = errors.Wrap(err, "rclone driver: transfer job error")
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  cTime,
		}, err
	}

	// the initial save when everything went ok
	if err := driver.storage.StoreJob(job); err != nil {
		err = errors.Wrap(err, "rclone driver: error starting transfer job")
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: txID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  cTime,
		}, err
	}

	// start separate dedicated process to periodically check the transfer progress
	go func() {
		// runs for as long as no end state or time out has been reached
		startTimeMs := time.Now().Nanosecond() / 1000
		timeout := driver.config.JobTimeout

		for {
			job, err := driver.storage.GetJob(txID)
			if err != nil {
				logger.Error().Err(err).Msgf("rclone driver: unable to retrieve transfer job with id: %v", txID)
				break
			}

			// check for end status first
			_, endStatusreached := txEndStatuses[job.TransferStatus.String()]
			if endStatusreached {
				logger.Info().Msgf("rclone driver: transfer job endstatus reached: %v", job.TransferStatus)
				break
			}

			// check for possible timeout and if true were done
			currentTimeMs := time.Now().Nanosecond() / 1000
			timePastMs := currentTimeMs - startTimeMs

			if timePastMs > timeout {
				logger.Info().Msgf("rclone driver: transfer job timed out: %vms (timeout = %v)", timePastMs, timeout)
				// set status to EXPIRED and save
				job.TransferStatus = datatx.Status_STATUS_TRANSFER_EXPIRED
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
				break
			}

			jobID := job.JobID
			type rcloneStatusReqJSON struct {
				JobID int64 `json:"jobid"`
			}
			rcloneStatusReq := &rcloneStatusReqJSON{
				JobID: jobID,
			}

			data, err := json.Marshal(rcloneStatusReq)
			if err != nil {
				logger.Error().Err(err).Msgf("rclone driver: marshalling request failed: %v", err)
				job.TransferStatus = datatx.Status_STATUS_INVALID
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
				break
			}

			transferFileMethod := "/job/status"

			u, err := url.Parse(driver.config.Endpoint)
			if err != nil {
				logger.Error().Err(err).Msgf("rclone driver: could not parse driver endpoint: %v", err)
				job.TransferStatus = datatx.Status_STATUS_INVALID
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
				break
			}
			u.Path = path.Join(u.Path, transferFileMethod)
			requestURL := u.String()

			req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(data))
			if err != nil {
				logger.Error().Err(err).Msgf("rclone driver: error framing post request: %v", err)
				job.TransferStatus = datatx.Status_STATUS_INVALID
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
				break
			}
			req.Header.Set("Content-Type", "application/json")
			req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)
			res, err := driver.client.Do(req)
			if err != nil {
				logger.Error().Err(err).Msgf("rclone driver: error sending post request: %v", err)
				job.TransferStatus = datatx.Status_STATUS_INVALID
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
				break
			}

			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				var errorResData rcloneHTTPErrorRes
				if err = json.NewDecoder(res.Body).Decode(&errorResData); err != nil {
					err = errors.Wrap(err, "rclone driver: error decoding response data")
					logger.Error().Err(err).Msgf("rclone driver: error reading response body: %v", err)
				}
				logger.Error().Err(err).Msgf("rclone driver: rclone request responded with error, status: %v, error: %v", errorResData.Status, errorResData.Error)
				job.TransferStatus = datatx.Status_STATUS_INVALID
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: save transfer job failed: %v", err)
				}
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
				logger.Error().Err(err).Msgf("rclone driver: error decoding response data: %v", err)
				break
			}

			if resData.Error != "" {
				logger.Error().Err(err).Msgf("rclone driver: rclone responded with error: %v", resData.Error)
				job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: error saving transfer job: %v", err)
					break
				}
				break
			}

			// transfer complete
			if resData.Finished && resData.Success {
				logger.Info().Msg("rclone driver: transfer job finished")
				job.TransferStatus = datatx.Status_STATUS_TRANSFER_COMPLETE
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: error saving transfer job: %v", err)
					break
				}
				break
			}

			// transfer completed unsuccessfully without error
			if resData.Finished && !resData.Success {
				logger.Info().Msgf("rclone driver: transfer job failed")
				job.TransferStatus = datatx.Status_STATUS_TRANSFER_FAILED
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: error saving transfer job: %v", err)
					break
				}
				break
			}

			// transfer not yet finished: continue
			if !resData.Finished {
				logger.Info().Msgf("rclone driver: transfer job in progress")
				job.TransferStatus = datatx.Status_STATUS_TRANSFER_IN_PROGRESS
				if err := driver.storage.StoreJob(job); err != nil {
					logger.Error().Err(err).Msgf("rclone driver: error saving transfer job: %v", err)
					break
				}
			}

			<-time.After(time.Millisecond * time.Duration(driver.config.JobStatusCheckInterval))
		}
	}()

	return &datatx.TxInfo{
		Id:     &datatx.TxId{OpaqueId: txID},
		Status: transferStatus,
		Ctime:  cTime,
	}, nil
}

// GetTransferStatus returns the status of the transfer with the specified job id.
func (driver *rclone) GetTransferStatus(ctx context.Context, transferID string) (*datatx.TxInfo, error) {
	job, err := driver.storage.GetJob(transferID)
	if err != nil {
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  nil,
		}, err
	}
	cTime, _ := strconv.ParseInt(job.Ctime, 10, 64)
	return &datatx.TxInfo{
		Id:     &datatx.TxId{OpaqueId: transferID},
		Status: job.TransferStatus,
		Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
	}, nil
}

// CancelTransfer cancels the transfer with the specified transfer id.
func (driver *rclone) CancelTransfer(ctx context.Context, transferID string) (*datatx.TxInfo, error) {
	job, err := driver.storage.GetJob(transferID)
	if err != nil {
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  nil,
		}, err
	}

	cTime, _ := strconv.ParseInt(job.Ctime, 10, 64)
	// rclone cancel may fail so remove job from model first to be sure
	transferRemovedMessage := ""
	if driver.config.RemoveTransferJobOnCancel {
		if err := driver.storage.DeleteJob(job); err != nil {
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: transferID},
				Status: datatx.Status_STATUS_INVALID,
				Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
			}, err
		}
		transferRemovedMessage = "(transfer job successfully removed)"
	}

	_, endStatusFound := txEndStatuses[job.TransferStatus.String()]
	if endStatusFound {
		err := errors.Wrapf(errors.New("rclone driver: job already in end state"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}

	// rcloneStop the rclone job/stop method json request
	type rcloneStopRequest struct {
		JobID int64 `json:"jobid"`
	}
	rcloneCancelTransferReq := &rcloneStopRequest{
		JobID: job.JobID,
	}

	data, err := json.Marshal(rcloneCancelTransferReq)
	if err != nil {
		err := errors.Wrapf(errors.New("rclone driver: error marshalling rclone job/stop req data"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}

	transferFileMethod := "/job/stop"

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		err := errors.Wrapf(errors.New("rclone driver: error parsing driver endpoint"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}
	u.Path = path.Join(u.Path, transferFileMethod)
	requestURL := u.String()

	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		err := errors.Wrapf(errors.New("rclone driver: error framing post request"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}
	req.Header.Set("Content-Type", "application/json")

	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

	res, err := driver.client.Do(req)
	if err != nil {
		err := errors.Wrapf(errors.New("rclone driver: error sending post request"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorResData rcloneHTTPErrorRes
		if err = json.NewDecoder(res.Body).Decode(&errorResData); err != nil {
			err := errors.Wrapf(errors.New("rclone driver: error decoding response data"), transferRemovedMessage)
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: transferID},
				Status: datatx.Status_STATUS_INVALID,
				Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
			}, err
		}
		err = errors.Wrap(errors.Errorf("%v, status: %v, error: %v", transferRemovedMessage, errorResData.Status, errorResData.Error), "rclone driver: rclone request responded with error")
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
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
		err := errors.Wrapf(errors.New("rclone driver: error decoding response data"), transferRemovedMessage)
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_INVALID,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, err
	}

	if resData.Error != "" {
		return &datatx.TxInfo{
			Id:     &datatx.TxId{OpaqueId: transferID},
			Status: datatx.Status_STATUS_TRANSFER_CANCEL_FAILED,
			Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
		}, errors.New(resData.Error)
	}

	// only update when job's not removed
	if !driver.config.RemoveTransferJobOnCancel {
		job.TransferStatus = datatx.Status_STATUS_TRANSFER_CANCELLED
		if err := driver.storage.StoreJob(job); err != nil {
			return &datatx.TxInfo{
				Id:     &datatx.TxId{OpaqueId: transferID},
				Status: datatx.Status_STATUS_INVALID,
				Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
			}, err
		}
	}

	return &datatx.TxInfo{
		Id:     &datatx.TxId{OpaqueId: transferID},
		Status: datatx.Status_STATUS_TRANSFER_CANCELLED,
		Ctime:  &typespb.Timestamp{Seconds: uint64(cTime)},
	}, nil
}

// RetryTransfer retries the transfer with the specified transfer ID.
// Note that tokens must still be valid.
func (driver *rclone) RetryTransfer(ctx context.Context, transferID string) (*datatx.TxInfo, error) {
	return driver.startJob(ctx, transferID, "", "", "", "", "", "")
}

func (driver *rclone) remotePathIsFolder(remote string, remotePath string, remoteToken string) (bool, error) {
	type rcloneListReqJSON struct {
		Fs     string `json:"fs"`
		Remote string `json:"remote"`
	}
	fs := fmt.Sprintf(":webdav,headers=\"x-access-token,%v\",url=\"%v\":", remoteToken, remote)
	rcloneReq := &rcloneListReqJSON{
		Fs:     fs,
		Remote: remotePath,
	}
	data, err := json.Marshal(rcloneReq)
	if err != nil {
		return false, errors.Wrap(err, "rclone: error marshalling rclone req data")
	}

	listMethod := "/operations/list"

	u, err := url.Parse(driver.config.Endpoint)
	if err != nil {
		return false, errors.Wrap(err, "rclone driver: error parsing driver endpoint")
	}
	u.Path = path.Join(u.Path, listMethod)
	requestURL := u.String()

	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		return false, errors.Wrap(err, "rclone driver: error framing post request")
	}
	req.Header.Set("Content-Type", "application/json")

	req.SetBasicAuth(driver.config.AuthUser, driver.config.AuthPass)

	res, err := driver.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "rclone driver: error sending post request")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errorResData rcloneHTTPErrorRes
		if err = json.NewDecoder(res.Body).Decode(&errorResData); err != nil {
			return false, errors.Wrap(err, "rclone driver: error decoding response data")
		}
		return false, errors.Wrap(errors.Errorf("status: %v, error: %v", errorResData.Status, errorResData.Error), "rclone driver: rclone request responded with error")
	}

	type item struct {
		Path     string `json:"Path"`
		Name     string `json:"Name"`
		Size     int64  `json:"Size"`
		MimeType string `json:"MimeType"`
		ModTime  string `json:"ModTime"`
		IsDir    bool   `json:"IsDir"`
	}
	type rcloneListResJSON struct {
		List []*item `json:"list"`
	}

	var resData rcloneListResJSON
	if err = json.NewDecoder(res.Body).Decode(&resData); err != nil {
		return false, errors.Wrap(err, "rclone driver: error decoding response data")
	}

	// a file will return one single item, the file, with path being the remote path and IsDir will be false
	if len(resData.List) == 1 && resData.List[0].Path == remotePath && !resData.List[0].IsDir {
		return false, nil
	}

	// in all other cases the remote path is a directory
	return true, nil
}

func (driver *rclone) extractEndpointInfo(ctx context.Context, targetURL string) (*endpoint, error) {
	if targetURL == "" {
		return nil, errtypes.BadRequest("datatx service: ref target is an empty uri")
	}

	uri, err := url.Parse(targetURL)
	if err != nil {
		return nil, errors.Wrap(err, "datatx service: error parsing target uri: "+targetURL)
	}

	m, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, errors.Wrap(err, "datatx service: error parsing target resource name")
	}

	var path string
	if m["name"] != nil {
		path = m["name"][0]
	}

	return &endpoint{
		filePath:       path,
		endpoint:       uri.Host + uri.Path,
		endpointScheme: uri.Scheme,
		token:          uri.User.String(),
	}, nil
}
