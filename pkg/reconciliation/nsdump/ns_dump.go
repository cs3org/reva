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
	Ctime       string    `json:"ctime"`
	Flags       string    `json:"flags"`
	Gid         string    `json:"gid"`
	Mtime       string    `json:"mtime"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Stime       string    `json:"stime"`
	Uid         string    `json:"uid"`
	XattrSysAcl string    `json:"xattr.sys.acl"`
	EntryType   EntryType `json:"entryType"`
	ID          string    `json:"id"`
}

func (e *NameSpaceEntry) UnmarshalJSON(data []byte) error {
	type n NameSpaceEntry // no methods, so no infinite recursion
	var aux struct {
		n
		CID *string `json:"cid"`
		FID *string `json:"fid"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch {
	case aux.CID != nil && aux.FID != nil:
		return errors.New("item: both cid and fid set")
	case aux.CID != nil:
		aux.ID, aux.EntryType = *aux.CID, EntryTypeFolder
	case aux.FID != nil:
		aux.ID, aux.EntryType = *aux.FID, EntryTypeFile
	default:
		return errors.New("item: neither cid nor fid set")
	}

	*e = NameSpaceEntry(aux.n)
	return nil
}

func (d *NameSpaceEntry) IsSysFolder() bool {
	return d.EntryType == EntryTypeFolder && strings.HasPrefix(d.Name, ".sys.")
}

func (d *NameSpaceEntry) IsSysFile() bool {
	return d.EntryType == EntryTypeFile && strings.HasPrefix(path.Base(path.Dir(d.Path)), ".sys.")
}

func parseNSInspectOutput(data []byte) (*NamespaceDump, error) {
	var raw []map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal ns-inspect input")
	}

	var entries []NameSpaceEntry
	for _, obj := range raw {
		var e NameSpaceEntry
		b, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(b, &e)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)

	}
	return &NamespaceDump{Entries: entries}, nil
}
