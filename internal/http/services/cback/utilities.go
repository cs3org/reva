package cback

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
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

func (s *svc) pathTrimmer(snapshotList []snapshotResponse, resp *backUpResponse) (string, string) {

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

func (s *svc) listSnapshots(userName string, backupID int) ([]snapshotResponse, error) {

	url := s.conf.APIURL + strconv.Itoa(backupID) + "/snapshots"
	requestType := "GET"
	responseData, err := s.Request(userName, url, requestType, nil)

	if err != nil {
		return nil, err
	}

	defer responseData.Close()

	/*Unmarshalling the JSON response into the Response struct*/
	responseObject := []snapshotResponse{}
	json.NewDecoder(responseData).Decode(&responseObject)

	return responseObject, nil
}

func (s *svc) matchBackups(userName, pathInput string) (*backUpResponse, error) {

	url := s.conf.APIURL
	requestType := "GET"
	responseData, err := s.Request(userName, url, requestType, nil)

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
