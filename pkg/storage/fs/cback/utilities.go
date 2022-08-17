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

package cback

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
)

type backUpResponse struct {
	Detail    string `json:"detail"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Substring string // Used in function
}

type snapshotResponse struct {
	Detail string   `json:"detail"`
	ID     string   `json:"id"`
	Time   string   `json:"time"`
	Paths  []string `json:"paths"`
}

type contents struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Mode   uint64  `json:"mode"`
	Mtime  float64 `json:"mtime"`
	Atime  float64 `json:"atime"`
	Ctime  float64 `json:"ctime"`
	Inode  uint64  `json:"inode"`
	Size   uint64  `json:"size"`
	Detail string  `json:"detail"`
}

type fsReturn struct {
	Type   provider.ResourceType
	Mtime  uint64
	Size   uint64
	Path   string
	Detail string
}

var permID = provider.ResourcePermissions{
	AddGrant:             false,
	CreateContainer:      false,
	Delete:               false,
	GetPath:              true,
	GetQuota:             true,
	InitiateFileDownload: true,
	InitiateFileUpload:   false,
	ListGrants:           true,
	ListContainer:        true,
	ListFileVersions:     true,
	ListRecycle:          false,
	Move:                 false,
	RemoveGrant:          false,
	PurgeRecycle:         false,
	RestoreFileVersion:   true,
	RestoreRecycleItem:   false,
	Stat:                 true,
	UpdateGrant:          false,
	DenyGrant:            false}

var checkSum = provider.ResourceChecksum{
	Sum:  "",
	Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
}

func mapReturn(fileType string) (provider.ResourceType, error) {
	/* This function can be changed accordingly, depending on the file type
	being return by the APIs */

	switch fileType {
	case "file":
		return provider.ResourceType_RESOURCE_TYPE_FILE, nil

	case "dir":
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER, nil

	default:
		return provider.ResourceType_RESOURCE_TYPE_INVALID, errtypes.NotFound("Resource type unrecognized")
	}
}

func (fs *cback) getRequest(userName, url string, reqType string, body io.Reader) (io.ReadCloser, error) {

	req, err := http.NewRequest(reqType, url, body)
	req.SetBasicAuth(userName, fs.conf.ImpersonatorToken)

	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	req.Header.Add("accept", `application/json`)

	resp, err := fs.client.Do(req)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {

		switch resp.StatusCode {

		case http.StatusNotFound:
			return nil, errtypes.NotFound("cback: resource not found")
		case http.StatusForbidden:
			return nil, errtypes.PermissionDenied("cback: user has no permissions to get the backup")
		case http.StatusBadRequest:
			return nil, errtypes.BadRequest("cback")
		default:
			return nil, errtypes.InternalError("cback: internal server error")
		}
	}

	return resp.Body, nil

}

func (fs *cback) listSnapshots(userName string, backupID int) ([]snapshotResponse, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots"
	responseData, err := fs.getRequest(userName, url, http.MethodGet, nil)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []snapshotResponse{}

	err = json.NewDecoder(responseData).Decode(&responseObject)

	if err != nil {
		return nil, err
	}

	return responseObject, nil
}

func (fs *cback) matchBackups(userName, pathInput string) (*backUpResponse, error) {

	url := fs.conf.APIURL + "/backups/"
	responseData, err := fs.getRequest(userName, url, http.MethodGet, nil)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []backUpResponse{}

	err = json.NewDecoder(responseData).Decode(&responseObject)

	if err != nil {
		return nil, err
	}

	if len(responseObject) == 0 {
		err = errors.New("no match found")
		return nil, err
	}

	for _, response := range responseObject {
		if response.Detail != "" {
			err = errors.New(response.Detail)
			return nil, err
		}

		if strings.Compare(pathInput, response.Source) == 0 {
			return &response, nil
		}
	}

	for _, response := range responseObject {
		if response.Detail != "" {
			err = errors.New(response.Detail)
			return nil, err
		}

		if strings.HasPrefix(pathInput, response.Source) {
			substr := strings.TrimPrefix(pathInput, response.Source)
			substr = strings.TrimLeft(substr, "/")
			response.Substring = substr
			return &response, nil
		}
	}

	/*If there is no error, but also no match found in the backup path the response is nil.
	This means that the LSFolder function will know that no match has been found using the exact path,
	and will therefore start checking if there is a substring of the backup job included in the inputted path*/
	return nil, nil
}

func (fs *cback) statResource(backupID int, snapID, userName, path, source string) (*fsReturn, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots/" + snapID + "/" + path + "?content=false"
	responseData, err := fs.getRequest(userName, url, http.MethodOptions, nil)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	responseObject := contents{}

	err = json.NewDecoder(responseData).Decode(&responseObject)

	if err != nil {
		return nil, err
	}

	m, err := mapReturn(responseObject.Type)

	if err != nil {
		return nil, err
	}

	retObject := fsReturn{
		Path:   source + "/" + snapID + strings.TrimPrefix(responseObject.Name, source),
		Type:   m,
		Mtime:  uint64(responseObject.Mtime),
		Size:   responseObject.Size,
		Detail: responseObject.Detail,
	}

	return &retObject, nil
}

func (fs *cback) fileSystem(backupID int, snapID, userName, path, source string) ([]*fsReturn, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots/" + snapID + "/" + path + "?content=true"
	responseData, err := fs.getRequest(userName, url, http.MethodOptions, nil)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []contents{}

	err = json.NewDecoder(responseData).Decode(&responseObject)

	if err != nil {
		return nil, err
	}

	resp := make([]*fsReturn, 0, len(responseObject))

	for _, response := range responseObject {

		m, err := mapReturn(response.Type)

		if err != nil {
			return nil, err
		}

		temp := fsReturn{
			Size:  response.Size,
			Type:  m,
			Mtime: uint64(response.Mtime),
			Path:  source + "/" + snapID + strings.TrimPrefix(response.Name, source),
		}

		resp = append(resp, &temp)
	}

	return resp, nil
}

func (fs *cback) timeConv(timeInput string) (int64, error) {
	tm, err := time.Parse(time.RFC3339, timeInput)

	if err != nil {
		return 0, err
	}

	epoch := tm.Unix()
	return epoch, nil
}

func (fs *cback) pathFinder(userName, path string) ([]string, error) {
	url := fs.conf.APIURL + "/backups/"
	responseData, err := fs.getRequest(userName, url, http.MethodGet, nil)
	matchFound := false

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []backUpResponse{}

	err = json.NewDecoder(responseData).Decode(&responseObject)

	if err != nil {
		return nil, err
	}

	returnString := make([]string, 0, len(responseObject))

	for _, response := range responseObject {
		if response.Detail != "" {
			err = errors.New(response.Detail)
			return nil, err
		}

		if strings.HasPrefix(response.Source, path) {
			substr := strings.TrimPrefix(response.Source, path)
			substr = strings.TrimLeft(substr, "/")
			temp := strings.Split(substr, "/")
			returnString = append(returnString, temp[0])
			matchFound = true
		}
	}

	if matchFound {
		return duplicateRemoval(returnString), nil
	}

	return nil, errtypes.NotFound("cback: resource not found")

}

func (fs *cback) pathTrimmer(snapshotList []snapshotResponse, resp *backUpResponse) (string, string) {

	var ssID, searchPath string

	for _, snapshot := range snapshotList {

		if snapshot.ID == resp.Substring {
			ssID = resp.Substring
			searchPath = resp.Source
			break

		} else if strings.HasPrefix(resp.Substring, snapshot.ID) {
			searchPath = strings.TrimPrefix(resp.Substring, snapshot.ID)
			searchPath = resp.Source + searchPath
			ssID = snapshot.ID
			break
		}
	}

	return ssID, searchPath

}

func duplicateRemoval(strSlice []string) []string {
	inList := make(map[string]bool)
	var list []string
	for _, str := range strSlice {
		if _, value := inList[str]; !value {
			inList[str] = true
			list = append(list, str)
		}
	}
	return list
}
