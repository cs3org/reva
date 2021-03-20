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

type APITokenManager struct {
	oidcApiToken            string
	oidcTokenExpirationTime time.Time
	TargetAPI               string
	OIDCTokenEndpoint       string
	ClientID                string
	ClientSecret            string
	Client                  *http.Client
	mu                      sync.Mutex
}

func (a *APITokenManager) renewAPIToken(ctx context.Context, forceRenewal bool) error {
	// Recieved tokens have an expiration time of 20 minutes.
	// Take a couple of seconds as buffer time for the API call to complete
	if forceRenewal || a.oidcTokenExpirationTime.Before(time.Now().Add(time.Second*time.Duration(2))) {
		token, expiration, err := a.getAPIToken(ctx)
		if err != nil {
			return err
		}

		a.mu.Lock()
		defer a.mu.Unlock()

		a.oidcApiToken = token
		a.oidcTokenExpirationTime = expiration
	}
	return nil
}

func (a *APITokenManager) getAPIToken(ctx context.Context) (string, time.Time, error) {

	params := url.Values{
		"grant_types": {"client_credentials"},
		"audience":    {a.TargetAPI},
	}

	httpReq, err := http.NewRequest("POST", a.OIDCTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq.SetBasicAuth(a.ClientID, a.ClientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

	httpRes, err := a.Client.Do(httpReq)
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

func (a *APITokenManager) SendAPIGetRequest(ctx context.Context, url string, forceRenewal bool) ([]interface{}, error) {
	err := a.renewAPIToken(ctx, forceRenewal)
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
	httpReq.Header.Set("Authorization", "Bearer "+a.oidcApiToken)

	httpRes, err := a.Client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode == http.StatusUnauthorized {
		// The token is no longer valid, try renewing it
		return a.SendAPIGetRequest(ctx, url, true)
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
