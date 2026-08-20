package nsdump

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/pkg/errors"
)

type NamespaceDump struct {
	Entries []NameSpaceEntry
}

type NSDumpClient interface {
	Dump(rootPath string, maxDepth int) (*NamespaceDump, error)
	Setup(config map[string]any) error
}

type EntryType string

const (
	EntryTypeFolder EntryType = "folder"
	EntryTypeFile   EntryType = "file"
)

type NameSpaceEntry struct {
	Ctime       string `json:"ctime"`
	Flags       string `json:"flags"`
	Gid         string `json:"gid"`
	Mtime       string `json:"mtime"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Stime       string `json:"stime"`
	Uid         string `json:"uid"`
	XattrSysAcl string `json:"xattr.sys.acl"`
	CID         string `json:"cid"`
	FID         string `json:"fid"`
}

func (e *NameSpaceEntry) EntryType() EntryType {
	if e.CID != "" {
		return EntryTypeFolder
	}
	return EntryTypeFile
}

func (e *NameSpaceEntry) ID() string {
	if e.CID != "" {
		return e.CID
	}
	return e.FID
}

func (e *NameSpaceEntry) IsSysFolder() bool {
	return e.EntryType() == EntryTypeFolder && strings.HasPrefix(e.Name, ".sys.")
}

func (e *NameSpaceEntry) IsSysFile() bool {
	return e.EntryType() == EntryTypeFile && strings.HasPrefix(path.Base(path.Dir(e.Path)), ".sys.")
}

func parseNSInspectOutput(data []byte) (*NamespaceDump, error) {
	var entries []NameSpaceEntry

	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal ns-inspect input")
	}

	return &NamespaceDump{Entries: entries}, nil
}
