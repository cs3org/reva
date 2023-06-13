package config

import (
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Command interface {
	isCommand()
}

type FieldByKey struct {
	Key string
}

func (FieldByKey) isCommand() {}

type FieldByIndex struct {
	Index int
}

func (FieldByIndex) isCommand() {}

func parseNext(key string) (Command, string, error) {
	// key = ".grpc.services.authprovider[1].address"

	key = strings.TrimSpace(key)

	// first character must be either "." or "["
	// unless the key is empty
	if key == "" {
		return nil, "", io.EOF
	}

	switch {
	case strings.HasPrefix(key, "."):
		tkn, next := split(key)
		return FieldByKey{Key: tkn}, next, nil
	case strings.HasPrefix(key, "["):
		tkn, next := split(key)
		index, err := strconv.ParseInt(tkn, 10, 64)
		if err != nil {
			return nil, "", errors.Wrap(err, "parsing error:")
		}
		return FieldByIndex{Index: int(index)}, next, nil
	}

	return nil, "", errors.New("parsing error: operator not recognised")
}

func split(key string) (token string, next string) {
	// key = ".grpc.services.authprovider[1].address"
	//         -> grpc
	// key = "[<i>].address"
	// 		   -> <i>
	if key == "" {
		return
	}

	i := -1
	s := key[0]
	key = key[1:]

	switch s {
	case '.':
		i = strings.IndexAny(key, ".[")
	case '[':
		i = strings.IndexByte(key, ']')
	}

	if i == -1 {
		return key, ""
	}

	if key[i] == ']' {
		return key[:i], key[i+1:]
	}
	return key[:i], key[i:]
}
