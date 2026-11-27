// Copyright 2018-2024 CERN
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

package ocdav

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// buildLnk assembles a minimal shell link with the given optional parts.
func buildLnk(flags, attrs uint32, idList []byte, linkInfoFlags uint32, name, rel string) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, uint32(lnkHeaderSize))
	b.Write(lnkCLSID)
	_ = binary.Write(&b, le, flags)
	_ = binary.Write(&b, le, attrs)
	b.Write(make([]byte, lnkHeaderSize-28))
	if flags&lnkHasLinkTargetIDList != 0 {
		_ = binary.Write(&b, le, uint16(len(idList)))
		b.Write(idList)
	}
	if flags&lnkHasLinkInfo != 0 {
		// LinkInfoSize, LinkInfoHeaderSize, LinkInfoFlags, 4 offsets, 4 bytes of payload
		_ = binary.Write(&b, le, uint32(32))
		_ = binary.Write(&b, le, uint32(28))
		_ = binary.Write(&b, le, linkInfoFlags)
		b.Write(make([]byte, 20))
	}
	writeStr := func(s string) {
		if flags&lnkIsUnicode != 0 {
			u := utf16.Encode([]rune(s))
			_ = binary.Write(&b, le, uint16(len(u)))
			_ = binary.Write(&b, le, u)
		} else {
			_ = binary.Write(&b, le, uint16(len(s)))
			b.WriteString(s)
		}
	}
	if flags&lnkHasName != 0 {
		writeStr(name)
	}
	if flags&lnkHasRelativePath != 0 {
		writeStr(rel)
	}
	return b.Bytes()
}

func TestParseShellLink(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantErr   bool
		wantRel   string
		wantAbs   bool
		wantIsDir bool
	}{
		{
			name: "unicode relative with idlist, linkinfo and name",
			data: buildLnk(lnkHasLinkTargetIDList|lnkHasLinkInfo|lnkHasName|lnkHasRelativePath|lnkIsUnicode,
				lnkFileAttributeDirectory, []byte{1, 2, 3, 4}, lnkInfoVolumeIDAndLocalBasePath, "a comment", `..\Projets\Été`),
			wantRel:   `..\Projets\Été`,
			wantAbs:   true,
			wantIsDir: true,
		},
		{
			name:    "ansi relative only",
			data:    buildLnk(lnkHasRelativePath, 0, nil, 0, "", `.\report.docx`),
			wantRel: `.\report.docx`,
		},
		{
			name:    "absolute only",
			data:    buildLnk(lnkHasLinkInfo|lnkIsUnicode, 0, nil, lnkInfoCommonNetworkRelativeLinkAndPathSuffix, "", ""),
			wantAbs: true,
		},
		{
			name:    "bad clsid",
			data:    append(append([]byte{0x4C, 0, 0, 0}, make([]byte, 16)...), make([]byte, 56)...),
			wantErr: true,
		},
		{
			name:    "truncated string",
			data:    buildLnk(lnkHasRelativePath|lnkIsUnicode, 0, nil, 0, "", `.\x`)[:lnkHeaderSize+3],
			wantErr: true,
		},
		{
			name:    "too short",
			data:    []byte("[InternetShortcut]\nURL=x"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl, err := parseShellLink(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if sl.RelativePath != tt.wantRel || sl.HasAbsoluteTarget != tt.wantAbs || sl.IsDir != tt.wantIsDir {
				t.Errorf("got %+v", sl)
			}
		})
	}
}
