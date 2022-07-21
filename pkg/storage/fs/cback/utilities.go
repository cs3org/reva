package cback

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type BackUpResponse struct {
	Detail string `json:"detail"`
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
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

func pathParser(path string) []string {
	var seperator string = "/"

	c := strings.Split(path, seperator)

	return c

}

func (fs *cback) getRequest(url string, reqType string, username string) (responseData []byte, erro error) {

	req, err := http.NewRequest(reqType, url, nil)
	req.SetBasicAuth(username, fs.conf.ImpersonatorToken)

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

func (fs *cback) getBackups(user *User) []string {

	url := "http://cback-portal-dev-01:8000/backups/"
	requestType := "GET"
	responseData, err := fs.getRequest(url, requestType, user.GetUsername())

	if err != nil {
		fmt.Printf("Invalid API request. Check backupID is valid.")
		log.Fatal(err)
	}

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []BackUpResponse{}
	json.Unmarshal([]byte(responseData), &responseObject)

	strArray := make([]string, len(responseObject))

	for i := range responseObject {
		if responseObject[i].Detail != "" {
			fmt.Printf(responseObject[i].Detail)
		}
		fmt.Printf("Username is: %v\n", responseObject[i].Name)
		fmt.Printf("Backup ID is: %d\n", responseObject[i].Id)
		strArray[i] = responseObject[i].Source
	}
	return strArray
}
