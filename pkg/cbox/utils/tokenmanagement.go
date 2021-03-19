package utils

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var expirationTime time.Time

func renewAPIToken(ctx context.Context, forceRenewal bool, targetAPI string, OIDCTokenEndpoint string, clientID string, clientSecret string, client *http.Client) (string, error) {
	// Recieved tokens have an expiration time of 20 minutes.
	// Take a couple of seconds as buffer time for the API call to complete
	var apiToken string
	var mutex = &sync.Mutex{}
	if forceRenewal || expirationTime.Before(time.Now().Add(time.Second*time.Duration(2))) {
		token, expiration, err := getAPIToken(ctx, targetAPI, OIDCTokenEndpoint, clientID, clientSecret, client)
		if err != nil {
			return "", err
		}

		mutex.Lock()
		defer mutex.Unlock()

		apiToken = token
		expirationTime = expiration
	}
	return apiToken, nil
}

func getAPIToken(ctx context.Context, targetAPI string, OIDCTokenEndpoint string, clientID string, clientSecret string, client *http.Client) (string, time.Time, error) {
	params := url.Values{
		"grant_type": {"client_credentials"},
		"audience":   {targetAPI},
	}

	httpReq, err := http.NewRequest("POST", OIDCTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq.SetBasicAuth(clientID, clientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

	httpRes, err := client.Do(httpReq)
	if err != nil {
		return "", time.Time{}, err
	}
	defer httpRes.Body.Close()

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return "", time.Time{}, err
	}
	if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
		return "", time.Time{}, errors.New("rest: get token endpoint returned " + httpRes.Status)
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", time.Time{}, err
	}

	expirationSecs := result["expires_in"].(float64)
	expirationTime := time.Now().Add(time.Second * time.Duration(expirationSecs))

	return result["access_token"].(string), expirationTime, nil
}

func SendAPIRequest(ctx context.Context, url string, forceRenewal bool, client *http.Client, targetAPI string, OIDCTokenEndpoint string, clientID string, clientSecret string) ([]interface{}, error) {
	token, err := renewAPIToken(ctx, forceRenewal, targetAPI, OIDCTokenEndpoint, clientID, clientSecret, client)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// We don't need to take the lock when reading apiToken, because if we reach here,
	// the token is valid at least for a couple of seconds. Even if another request modifies
	// the token and expiration time while this request is in progress, the current token will still be valid.
	httpReq.Header.Set("Authorization", "Bearer "+token)

	httpRes, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode == http.StatusUnauthorized {
		// The token is no longer valid, try renewing it
		return SendAPIRequest(ctx, url, true, client, targetAPI, OIDCTokenEndpoint, clientID, clientSecret)
	}
	if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
		return nil, errors.New("rest: API request returned " + httpRes.Status)
	}

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	responseData, ok := result["data"].([]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	return responseData, nil
}
