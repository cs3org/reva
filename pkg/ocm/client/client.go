package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/pkg/errors"
)

// ErrTokenInvalid is the error returned by the invite-accepted
// endpoint when the token is not valid.
var ErrTokenInvalid = errors.New("the invitation token is invalid")

// ErrServiceNotTrusted is the error returned by the invite-accepted
// endpoint when the service is not trusted to accept invitations.
var ErrServiceNotTrusted = errors.New("service is not trusted to accept invitations")

type OCMClient struct {
	client *http.Client
}

type Config struct {
	Timeout  time.Duration
	Insecure bool
}

func New(c *Config) *OCMClient {
	return &OCMClient{
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(c.Timeout),
			rhttp.Insecure(c.Insecure),
		),
	}
}

type InviteAcceptedRequest struct {
	UserID            string `json:"userID"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	RecipientProvider string `json:"recipientProvider"`
	Token             string `json:"token"`
}

type User struct {
	UserID string `json:"userID"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

func (r *InviteAcceptedRequest) toJSON() (io.Reader, error) {
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(r); err != nil {
		return nil, err
	}
	return &b, nil
}

// InviteAccepted informs the sender that the invitation was accepted to start sharing
// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1invite-accepted/post
func (c *OCMClient) InviteAccepted(ctx context.Context, endpoint string, r *InviteAcceptedRequest) (*User, error) {
	url, err := url.JoinPath(endpoint, "invite-accepted")
	if err != nil {
		return nil, err
	}

	body, err := r.toJSON()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error doing request")
	}
	defer resp.Body.Close()

	return c.parseInviteAcceptedResponse(resp)
}

func (c *OCMClient) parseInviteAcceptedResponse(r *http.Response) (*User, error) {
	switch r.StatusCode {
	case http.StatusOK:
		var u User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			return nil, errors.Wrap(err, "error decoding response body")
		}
		return &u, nil
	case http.StatusBadRequest:
		return nil, ErrTokenInvalid
	case http.StatusForbidden:
		return nil, ErrServiceNotTrusted
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	return nil, errtypes.InternalError(string(body))
}
