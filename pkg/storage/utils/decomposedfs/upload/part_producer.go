package upload

import (
	"bytes"
	"io"
)

// partProducer converts a stream of bytes from the reader into a stream memory of buffers
type partProducer struct {
	parts chan<- *bytes.Buffer
	done  chan struct{}
	err   error
	r     io.Reader
}

func (spp *partProducer) produce(partSize int64) {
	for {
		file, err := spp.nextPart(partSize)
		if err != nil {
			spp.err = err
			close(spp.parts)
			return
		}
		if file == nil {
			close(spp.parts)
			return
		}
		select {
		case spp.parts <- file:
		case <-spp.done:
			close(spp.parts)
			return
		}
	}
}

func (spp *partProducer) nextPart(size int64) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)

	limitedReader := io.LimitReader(spp.r, size)
	n, err := buf.ReadFrom(limitedReader)
	if err != nil {
		return nil, err
	}

	// If the entire request body is read and no more data is available,
	// buf.ReadFrom returns 0 since it is unable to read any bytes. In that
	// case, we can close the partProducer.
	if n == 0 {
		return nil, nil
	}

	return buf, nil
}
