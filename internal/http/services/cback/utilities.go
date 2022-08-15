package cback

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cs3org/reva/pkg/errtypes"
)

type backUpResponse struct {
	Detail    string `json:"detail"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Substring string
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

	url := s.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots"
	responseData, err := s.getRequest(userName, url, http.MethodGet, nil)

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

func (s *svc) matchBackups(userName, pathInput string) (*backUpResponse, error) {

	url := s.conf.APIURL + "/backups/"
	responseData, err := s.getRequest(userName, url, http.MethodGet, nil)

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

func (s *svc) checkFileType(backupID int, snapID, userName, path, source string) error {

	url := s.conf.APIURL + "/backups/" + strconv.Itoa(backupID) + "/snapshots/" + snapID + "/" + path + "?content=false"

	responseData, err := s.getRequest(userName, url, http.MethodOptions, nil)

	if err != nil {
		return err
	}

	defer responseData.Close()

	return nil
}

func (s *svc) getRequest(userName, url string, reqType string, body io.Reader) (io.ReadCloser, error) {

	req, err := http.NewRequest(reqType, url, body)

	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(userName, s.conf.ImpersonatorToken)

	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	req.Header.Add("accept", `application/json`)

	resp, err := s.client.Do(req)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 && resp.StatusCode >= 300 {

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
