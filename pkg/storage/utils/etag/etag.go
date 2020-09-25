// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package etag

import (
	"crypto/md5"
	"fmt"
	"io"
	"regexp"
)

var (
	eosMtimeEtag    = regexp.MustCompile(`^(?P<inode>[\w0-9]+):(?P<mtime>[0-9]+\.?[0-9]*)$`)
	eosChecksumEtag = regexp.MustCompile(`^(?P<inode>[0-9]+):(?P<checksum>[\w0-9]{8})$`)
)

// This function handles multiple types of etags returned by EOS and other storage
// drivers. These can have the following formats:
// (https://github.com/cern-eos/eos/blob/master/namespace/utils/Etag.cc)
// - EOS directories: directory-id:mtime_sec:mtime_nanosec
// - EOS files: inode:checksum-length-8 or inode:mtime_sec or md5 checksum
// - Other drivers: md5 checksum
//
// We use the inodes, directory IDs and checksum to generate a new md5 checksum
// and append the latest mtime of the individual etags if available, so that we
// maintain the same format as the original etags. This is needed as different
// clients such as S3 expect these to follow the specified format.

func CombineEtags(etags []string) string {
	h := md5.New()
	var mtime string
	for _, e := range etags {
		m := findEtagMatch(e)
		if m["inode"] != "" {
			_, _ = io.WriteString(h, m["inode"])
		}
		if m["checksum"] != "" {
			_, _ = io.WriteString(h, m["checksum"])
		}
		if m["mtime"] != "" && m["mtime"] > mtime {
			mtime = m["mtime"]
		}
	}

	etag := fmt.Sprintf("%x", h.Sum(nil))
	if mtime != "" {
		etag = fmt.Sprintf("%s:%s", etag[:8], mtime)
	}
	return fmt.Sprintf("\"%s\"", etag)
}

func getEtagParams(regEx *regexp.Regexp, etag string) map[string]string {
	m := make(map[string]string)
	match := regEx.FindStringSubmatch(etag)
	for i, name := range regEx.SubexpNames() {
		if i > 0 && i < len(match) {
			m[name] = match[i]
		}
	}
	return m
}

func findEtagMatch(etag string) map[string]string {
	if m := getEtagParams(eosChecksumEtag, etag); len(m) > 0 {
		return m
	} else if m = getEtagParams(eosMtimeEtag, etag); len(m) > 0 {
		return m
	} else {
		m = make(map[string]string)
		m["checksum"] = etag
		return m
	}
}
