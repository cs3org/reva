// Copyright 2018-2026 CERN
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

package nsdump

import (
	"bytes"
	"fmt"
	"testing"
)

// benchDumpJSON builds what an eos-ns-inspect scan prints for n entries: one
// folder for every nine files, under three folder levels. The objects carry the
// key set of a real dump, because the parser walks every key of every object.
// The text is written out directly: a dump of a million entries is hundreds of
// megabytes, which is too much to hold twice.
func benchDumpJSON(n int) []byte {
	const root = "/eos/project/c/cernbox"

	var buf bytes.Buffer
	buf.Grow(n * 900)
	buf.WriteByte('[')

	for i := range n {
		if i > 0 {
			buf.WriteByte(',')
		}
		folder := fmt.Sprintf("%s/g%d/s%d/t%d", root, i%10, (i/10)%10, (i/100)%10)
		acl := fmt.Sprintf("egroup:group%d:rwx,u:user%d:rx,u:user%d:rwx", i%97, i%89, i%83)

		if i%10 == 0 {
			fmt.Fprintf(&buf, `{"cid":"%d","ctime":"1786473688.310186487","flags":"1",`+
				`"gid":"2763","mode":"42700","mtime":"1786474621.140556072","name":"t%d",`+
				`"parent_id":"348","path":"%s/","stime":"1786474621.140556072",`+
				`"tree_size":"1886117666","uid":"173503","xattr.sys.acl":"%s",`+
				`"xattr.sys.allow.oc.sync":"1","xattr.sys.eos.btime":"1677602740.101786072",`+
				`"xattr.sys.forced.atomic":"1","xattr.sys.forced.blocksize":"4k",`+
				`"xattr.sys.forced.checksum":"adler","xattr.sys.forced.layout":"replica",`+
				`"xattr.sys.forced.nstripes":"2","xattr.sys.forced.space":"default",`+
				`"xattr.sys.mask":"700","xattr.sys.mtime.propagation":"1",`+
				`"xattr.sys.owner.auth":"*","xattr.sys.recycle":"/eos/homedev/proc/recycle/",`+
				`"xattr.sys.versioning":"10"}`,
				4000000+i, (i/100)%10, folder, acl)
			continue
		}

		fmt.Fprintf(&buf, `{"atime":"1784818711.61715410","ctime":"1784818711.61714940",`+
			`"fid":"%d","flags":"644","gid":"2763","layout_id":"1048850","link_name":"",`+
			`"locations":"413,416","mtime":"1784818711.62072292","name":"file%d.txt",`+
			`"path":"%s/file%d.txt","pid":"4251882","size":"66","stime":"0.0","uid":"173503",`+
			`"unlink_locations":"","xattr.sys.acl":"%s",`+
			`"xattr.sys.eos.btime":"1784818711.61714940","xattr.sys.fs.tracking":"+413+416",`+
			`"xattr.sys.fusex.state":"","xattr.sys.utrace":"f81e1fe0-86a6-11f1-afee-fa163e35f83a",`+
			`"xattr.sys.vtrace":"[Thu Jul 23 16:58:31 2026] uid:173503[jgeens] gid:2763[it] `+
			`tident:jgeens.1:1@[2001:1458:d00:16::33d] name:jgeens dn: prot:https `+
			`app:http/reva_write host:cbox-ocisdev-diogo.cern.ch domain:cern.ch geo: sudo:0 `+
			`trace: onbehalf:","xs":"fcd91616"}`,
			110000000+i, i, folder, i, acl)
	}

	buf.WriteByte(']')
	return buf.Bytes()
}

// The dump of a whole space goes through this parser one time per run, so its
// cost is the price of getting the namespace in.
func BenchmarkParseNSInspectOutput(b *testing.B) {
	// the data is built inside the sub-benchmark, so running one size does not
	// generate the others
	for _, entries := range []int{1000, 10000, 100000, 1000000} {
		b.Run(fmt.Sprintf("%d entries", entries), func(b *testing.B) {
			data := benchDumpJSON(entries)

			// a parser that gives up early would measure nothing
			dump, err := parseNSInspectOutput(data)
			if err != nil {
				b.Fatalf("parsing %d entries: %v", entries, err)
			}
			if len(dump.Entries) != entries {
				b.Fatalf("parsed %d entries, want %d", len(dump.Entries), entries)
			}
			dump = nil

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := parseNSInspectOutput(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
