package cback

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type BackUpResponse struct {
	Detail    string `json:"detail"`
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Substring string //Used in function
}

type SnapshotResponse struct {
	Detail string   `json:"detail"`
	Id     string   `json:"id"`
	Time   string   `json:"time"`
	Paths  []string `json:"paths"`
}

type Contents struct {
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

type FsReturn struct {
	Type  int
	Mtime uint64
	Size  uint64
	Path  string
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

func getPermID() *provider.ResourcePermissions {
	return &permID
}

func mapReturn(fileType string) (int, error) {
	m := make(map[string]int)

	m["dir"] = 2
	m["file"] = 1

	if m[fileType] == 0 {
		return 0, errors.New("FileType not recognized")
	}

	return m[fileType], nil

}

func (fs *cback) getRequest(userName, url string, reqType string) (responseData []byte, erro error) {

	req, err := http.NewRequest(reqType, url, nil)
	req.SetBasicAuth(userName, fs.conf.ImpersonatorToken)

	if err != nil {
		fmt.Println("Error!")
	}

	req.Header.Add("accept", `application/json`)

	// Send request using HTTP Client
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error!")
	}

	responseData, erro = ioutil.ReadAll(resp.Body)
	return

}

func (fs *cback) listSnapshots(userName string, backupId int) []SnapshotResponse {

	url := "http://cback-portal-dev-01:8000/backups/" + strconv.Itoa(backupId) + "/snapshots"
	requestType := "GET"
	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		fmt.Printf("Invalid API request. Check backupID is valid.")
		log.Fatal(err)
	}

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []SnapshotResponse{}
	json.Unmarshal([]byte(responseData), &responseObject)

	return responseObject
}

func (fs *cback) matchBackups(userName, pathInput string) *BackUpResponse {

	url := "http://cback-portal-dev-01:8000/backups/"
	requestType := "GET"
	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		fmt.Printf("Invalid API request. Check backupID is valid.")
		log.Fatal(err)

	} else {
		/*Unmarshalling the JSON response into the Response struct*/
		responseObject := []BackUpResponse{}
		json.Unmarshal([]byte(responseData), &responseObject)

		if len(responseObject) == 0 {
			err = errors.New("no match")

		} else {

			for i := range responseObject {
				if responseObject[i].Detail != "" {
					fmt.Print(responseObject[i].Detail)
				}

				if strings.Compare(pathInput, responseObject[i].Source) == 0 {
					return &responseObject[i]
				}
			}

			for i := range responseObject {
				if responseObject[i].Detail != "" {
					fmt.Print(responseObject[i].Detail)
				}

				if strings.HasPrefix(pathInput, responseObject[i].Source) {
					substr := strings.TrimPrefix(pathInput, responseObject[i].Source)
					substr = strings.TrimLeft(substr, "/")
					responseObject[i].Substring = substr
					return &responseObject[i]
				}
			}
		}
	}
	return nil
}

func (fs *cback) fileSystem(backupId int, snapId, userName string, path string) []FsReturn {

	url := "http://cback-portal-dev-01:8000/backups/" + strconv.Itoa(backupId) + "/snapshots/" + snapId + "/" + path + "?content=true"
	requestType := "OPTIONS"

	responseData, err := fs.getRequest(userName, url, requestType)

	if err != nil {
		fmt.Printf("Invalid API request. Check backupID is valid.")
		log.Fatal(err)
	}

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []Contents{}
	json.Unmarshal([]byte(responseData), &responseObject)

	resp := make([]FsReturn, len(responseObject))

	for i := range responseObject {

		m, _ := mapReturn(responseObject[i].Type)

		/*fmt.Printf("\nName is: %v\n", responseObject[i].Name)
		fmt.Printf("Type is: %d\n", m)
		fmt.Printf("Time is: %v", uint64(responseObject[i].Mtime))
		fmt.Printf("\n")*/

		resp[i].Size = responseObject[i].Size
		resp[i].Type = m
		resp[i].Mtime = uint64(responseObject[i].Mtime)
		resp[i].Path = responseObject[i].Name

	}
	return resp
}
