package nsdump

import (
	"encoding/json"
	"io"
	"iter"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// NSDumpClient gives the entries of a namespace one at a time. The dump of a
// big space does not fit in memory, so the caller must not collect them.
type NSDumpClient interface {
	// Dump does the work when the caller ranges over the result. A failure
	// gives one error and ends the sequence.
	Dump(rootPath string, maxDepth int) iter.Seq2[NameSpaceEntry, error]
	Setup(config any) error
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

// nsInspectCommand builds the scan that the memory and the chunked client run.
func nsInspectCommand(instance, rootPath string, maxDepth int, ignoreFiles bool) *exec.Cmd {
	args := []string{"scan", "--path", rootPath}
	if maxDepth > 0 {
		args = append(args, "--maxdepth", strconv.Itoa(maxDepth))
	}
	args = append(args,
		"--members", instance+"-qdb:7777",
		"--password-file", "/keytabs/"+instance+"_keytab",
		"--json",
	)
	if ignoreFiles {
		args = append(args, "--no-files")
	}
	return exec.Command("/usr/bin/eos-ns-inspect", args...)
}

// parseNSInspect decodes the JSON array that eos-ns-inspect prints one element
// at a time, so only the entry that is being compared is in memory.
func parseNSInspect(r io.Reader) iter.Seq2[NameSpaceEntry, error] {
	return func(yield func(NameSpaceEntry, error) bool) {
		dec := json.NewDecoder(r)
		tok, err := dec.Token()
		if err != nil {
			yield(NameSpaceEntry{}, errors.Wrap(err, "failed to read the start of the ns-inspect output"))
			return
		}
		if tok != json.Delim('[') {
			yield(NameSpaceEntry{}, errors.Errorf("ns-inspect output is not a JSON array, it starts with %v", tok))
			return
		}

		for dec.More() {
			var e NameSpaceEntry
			if err := dec.Decode(&e); err != nil {
				yield(NameSpaceEntry{}, errors.Wrap(err, "failed to unmarshal ns-inspect input"))
				return
			}
			if !yield(e, nil) {
				return
			}
		}

		// More also stops at the end of the input, so a dump that was cut off
		// after a complete entry is only found here.
		if _, err := dec.Token(); err != nil {
			yield(NameSpaceEntry{}, errors.Wrap(err, "failed to read the end of the ns-inspect output"))
		}
	}
}
