package antivirus

import (
	"io"
	"net/http"
	"net/url"
	"time"

	ic "github.com/egirna/icap-client"
)

// NewICAP returns a Scanner talking to an ICAP server
func NewICAP(icapURL string, icapService string, timeout time.Duration) (ICAP, error) {
	endpoint, err := url.Parse(icapURL)
	if err != nil {
		return ICAP{}, err
	}

	endpoint.Scheme = "icap"
	endpoint.Path = icapService

	return ICAP{
		client: &ic.Client{
			Timeout: timeout,
		},
		endpoint: endpoint.String(),
	}, nil
}

// ICAP is a Scanner talking to an ICAP server
type ICAP struct {
	client   *ic.Client
	endpoint string
}

// Scan to fulfill Scanner interface
func (s ICAP) Scan(file io.Reader) (ScanResult, error) {
	httpReq, err := http.NewRequest(http.MethodGet, "http://localhost", file)
	if err != nil {
		return ScanResult{}, err
	}

	req, err := ic.NewRequest(ic.MethodREQMOD, s.endpoint, httpReq, nil)
	if err != nil {
		return ScanResult{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return ScanResult{}, err
	}

	_, infected := resp.Header["X-Infection-Found"]

	return ScanResult{
		Infected: infected,
	}, nil
}
