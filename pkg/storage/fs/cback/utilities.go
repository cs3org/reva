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
)

type backUpResponse struct {
	Detail    string `json:"detail"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Substring string //Used in function
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
	Type   int
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

func mapReturn(fileType string) (int, error) {
	/* This function can be changed accordingly, depending on the file type
	being return by the APIs */

	m := make(map[string]int)

	m["dir"] = 2
	m["file"] = 1

	if m[fileType] == 0 {
		return 0, errors.New("FileType not recognized")
	}

	return m[fileType], nil

}

func (fs *cback) getRequest(userName, url string, reqType string) (io.ReadCloser, error) {

	req, err := http.NewRequest(reqType, url, nil)
	req.SetBasicAuth(userName, fs.conf.ImpersonatorToken)

	if err != nil {
		return nil, err
	}

	req.Header.Add("accept", `application/json`)

	client := &http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (fs *cback) listSnapshots(userName string, backupID int) ([]snapshotResponse, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots"
	requestType := "GET"
	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []snapshotResponse{}
	json.NewDecoder(responseData).Decode(&responseObject)

	return responseObject, nil
}

func (fs *cback) matchBackups(userName, pathInput string) (*backUpResponse, error) {

	url := fs.conf.APIURL + "/backups/"
	requestType := "GET"
	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []backUpResponse{}
	json.NewDecoder(responseData).Decode(&responseObject)

	if len(responseObject) == 0 {
		err = errors.New("no match found")
		return nil, err
	}

	for i := range responseObject {
		if responseObject[i].Detail != "" {
			err = errors.New(responseObject[i].Detail)
			return nil, err
		}

		if strings.Compare(pathInput, responseObject[i].Source) == 0 {
			return &responseObject[i], nil
		}
	}

	for i := range responseObject {
		if responseObject[i].Detail != "" {
			err = errors.New(responseObject[i].Detail)
			return nil, err
		}

		if strings.HasPrefix(pathInput, responseObject[i].Source) {
			substr := strings.TrimPrefix(pathInput, responseObject[i].Source)
			substr = strings.TrimLeft(substr, "/")
			responseObject[i].Substring = substr
			return &responseObject[i], nil
		}
	}

	return nil, nil
}

func (fs *cback) statResource(backupID int, snapID, userName, path, source string) (*fsReturn, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots/" + snapID + "/" + path + "?content=false"
	requestType := "OPTIONS"

	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	responseObject := contents{}
	json.NewDecoder(responseData).Decode(&responseObject)

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

func (fs *cback) fileSystem(backupID int, snapID, userName, path, source string) ([]fsReturn, error) {

	url := fs.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots/" + snapID + "/" + path + "?content=true"
	requestType := "OPTIONS"

	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []contents{}
	json.NewDecoder(responseData).Decode(&responseObject)

	resp := make([]fsReturn, len(responseObject))

	for i, response := range responseObject {

		m, err := mapReturn(response.Type)

		if err != nil {
			return nil, err
		}
		/*fmt.Printf("\nName is: %v\n", responseObject[i].Name)
		fmt.Printf("Type is: %d\n", m)
		fmt.Printf("Time is: %v", uint64(responseObject[i].Mtime))
		fmt.Printf("\n")*/

		resp[i].Size = response.Size
		resp[i].Type = m
		resp[i].Mtime = uint64(response.Mtime)
		resp[i].Path = source + "/" + snapID + strings.TrimPrefix(response.Name, source)

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
	requestType := "GET"
	responseData, err := fs.getRequest(userName, url, requestType)
	matchFound := false

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []backUpResponse{}
	json.NewDecoder(responseData).Decode(&responseObject)

	returnString := make([]string, len(responseObject))

	for index, response := range responseObject {
		if response.Detail != "" {
			err = errors.New(response.Detail)
			return nil, err
		}

		if strings.HasPrefix(response.Source, path) {
			substr := strings.TrimPrefix(response.Source, path)
			substr = strings.TrimLeft(substr, "/")
			temp := strings.Split(substr, "/")
			returnString[index] = temp[0]
			matchFound = true
		}
	}

	if matchFound {
		return returnString, nil
	}

	err = errors.New("no match found")
	return nil, err

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
