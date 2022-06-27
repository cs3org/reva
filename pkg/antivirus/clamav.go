package antivirus

import (
	"github.com/dutchcoders/go-clamd"
	"io"
)

func NewClamAV() *ClamAV {
	return &ClamAV{
		clamd: clamd.NewClamd(clamavSocket),
	}
}

type ClamAV struct {
	clamd *clamd.Clamd
}

func (s *ClamAV) Scan(file io.Reader) (*ScanResult, error) {
	ch, err := s.clamd.ScanStream(file, make(chan bool))
	if err != nil {
		return nil, err
	}

	r := <-ch

	return &ScanResult{
		Infected: r.Status == clamd.RES_FOUND,
	}, nil
}
