package nsdump

import (
	"fmt"
	"os"
)

type EOSFileNSInspect struct {
	file string
}

type EOSFileNSInspectConfig struct {
	File string
}

func (e *EOSFileNSInspect) Setup(config any) error {
	if c, ok := config.(EOSFileNSInspectConfig); ok {
		e.file = c.File
		return nil
	}
	return fmt.Errorf("requires a config of type %t", EOSFileNSInspectConfig{})
}

func (e *EOSFileNSInspect) Dump(rootPath string, maxDepth int) (*NamespaceDump, error) {
	contents, err := os.ReadFile(e.file)
	if err != nil {
		return nil, err
	}

	return parseNSInspectOutput(contents)
}
