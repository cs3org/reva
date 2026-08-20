package reconciliation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/errors"
)

type NSDumpResponse struct {
	entries []NameSpaceEntry
}

type NSDumpClient interface {
	Dump(rootPath string, maxDepth int) (NSDumpResponse, error)
}

type EOSMemoryNSInspect struct {
	cfg EOSNSInspectConfig
}

type EOSNSInspectConfig struct {
	maxDepth    int
	ignoreFiles bool
	instance    string
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

func NewEOSMemoryNSInspect() (*EOSMemoryNSInspect, error) {
	return &EOSMemoryNSInspect{}, nil
}

func (e *EOSMemoryNSInspect) Dump(rootPath string, maxDepth int) (*NSDumpResponse, error) {

	noFilesFlag := ""
	if e.cfg.ignoreFiles {
		noFilesFlag = " --no-files"
	}

	maxDepthFlag := ""
	if maxDepth > 0 {
		maxDepthFlag = fmt.Sprintf(" --maxdepth %d", maxDepth)
	}

	args := fmt.Sprintf(
		"scan --path %s %s --members %s-qdb:7777 --password-file /keytabs/%s_keytab --json%s",
		rootPath,
		maxDepthFlag,
		e.cfg.instance,
		e.cfg.instance,
		noFilesFlag,
	)

	cmd := exec.Command("/usr/bin/eos-ns-inspect", strings.Split(args, " ")...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return parseNSInspectOutput(&stdout)
}

func parseNSInspectOutput(data *bytes.Buffer) (*NSDumpResponse, error) {
	var raw []map[string]interface{}
	if err := json.Unmarshal(data.Bytes(), &raw); err != nil {
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
	return &NSDumpResponse{entries: entries}, nil
}
