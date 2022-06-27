package antivirus

import (
	ic "github.com/egirna/icap-client"
	"io"
	"net/http"
	"net/url"
)

func NewIcap() (*Icap, error) {
	endpoint, err := url.Parse(icapUrl)
	if err != nil {
		return nil, err
	}

	endpoint.Scheme = "icap"
	endpoint.Path = icapService

	return &Icap{
		client: &ic.Client{
			Timeout: timeout,
		},
		endpoint: endpoint.String(),
	}, nil
}

type Icap struct {
	client   *ic.Client
	endpoint string
}

func (s *Icap) Scan(file io.Reader) (*ScanResult, error) {
	httpReq, err := http.NewRequest(http.MethodGet, "http://localhost", file)
	if err != nil {
		return nil, err
	}

	req, err := ic.NewRequest(ic.MethodREQMOD, s.endpoint, httpReq, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	_, infected := resp.Header["X-Infection-Found"]

	return &ScanResult{
		Infected: infected,
	}, nil
}
