package nsdump

import (
	"os"

	"github.com/pkg/errors"
)

type EOSFileNSInspect struct {
	file string
}

func (e *EOSFileNSInspect) Setup(config map[string]any) error {
	v, ok := config["file"]
	if !ok {
		return errors.New("file parameter must be present")
	}
	s, ok := v.(string)
	if !ok {
		return errors.New("file parameter must be a string representing a file path")
	}
	e.file = s
	return nil
}

func (e *EOSFileNSInspect) Dump(rootPath string, maxDepth int) (*NamespaceDump, error) {
	contents, err := os.ReadFile(e.file)
	if err != nil {
		return nil, err
	}

	return parseNSInspectOutput(contents)
}
