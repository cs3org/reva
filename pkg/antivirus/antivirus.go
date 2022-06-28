package antivirus

import (
	"io"
	"time"
)

// TODO: make configurable
var (
	_timeout      = 300 * time.Second
	_icapURL      = "icap://127.0.0.1:1344"
	_icapService  = "avscan"
	_clamavSocket = "/run/clamav/clamd.ctl" // "/tmp/clamd.socket"
)

// Scanner is an abstraction for the actual virus scan
type Scanner interface {
	Scan(file io.Reader) (ScanResult, error)
}

// ScanResult contains result about the scan
type ScanResult struct {
	Infected    bool
	Description string
}

// New returns an Antivirus
func New(typ string) (Scanner, error) {
	var (
		scanner Scanner
		err     error
	)

	switch typ {
	default:
		// TODO: error instead of fallback
		fallthrough
	case "clamav":
		scanner = NewClamAV(_clamavSocket)
	case "icap":
		scanner, err = NewICAP(_icapURL, _icapService, _timeout)
	}

	return scanner, err
}
