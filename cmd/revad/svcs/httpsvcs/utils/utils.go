package utils

import typespb "github.com/cernbox/go-cs3apis/cs3/types"
import "time"

func TSToUnixNano(ts *typespb.Timestamp) uint64 {
	return uint64(time.Unix(int64(ts.Seconds), int64(ts.Nanos)).UnixNano())
}

func TSToTime(ts *typespb.Timestamp) time.Time {
	return time.Unix(int64(ts.Seconds), int64(ts.Nanos))
}
