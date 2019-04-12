package utils

import typespb "github.com/cernbox/go-cs3apis/cs3/types"

// UnixNanoToTS converts a unix nano time to a valid cs3 Timestamp.
func UnixNanoToTS(epoch uint64) *typespb.Timestamp {
	seconds := epoch / 1000000000
	nanos := epoch * 1000000000
	ts := &typespb.Timestamp{
		Nanos:   uint32(nanos),
		Seconds: seconds,
	}
	return ts
}
