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
	"errors"
	"unicode/utf16"
)

// This file implements the minimal subset of the Shell Link Binary File Format
// (MS-SHLLINK) needed to extract the target of a Windows .lnk shortcut.
// See https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-shllink

const (
	lnkHeaderSize = 0x4C

	lnkHasLinkTargetIDList = 1 << 0
	lnkHasLinkInfo         = 1 << 1
	lnkHasName             = 1 << 2
	lnkHasRelativePath     = 1 << 3
	lnkIsUnicode           = 1 << 7

	lnkFileAttributeDirectory = 0x10

	lnkInfoVolumeIDAndLocalBasePath               = 1 << 0
	lnkInfoCommonNetworkRelativeLinkAndPathSuffix = 1 << 1
)

// lnkCLSID is the LinkCLSID every shell link header must carry,
// 00021401-0000-0000-C000-000000000046 in its on-disk (mixed-endian) form.
var lnkCLSID = []byte{0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}

var errInvalidLnk = errors.New("ocdav: not a valid shell link")

// shellLink holds the parts of a .lnk file relevant to resolve its target.
type shellLink struct {
	// RelativePath is the target relative to the location of the .lnk, with
	// Windows separators, e.g. `..\docs\report.docx`. Empty if not present.
	RelativePath string
	// HasAbsoluteTarget is true when the link carries a local base path or a
	// network (UNC) path, i.e. a target that is not relative to the link.
	HasAbsoluteTarget bool
	// IsDir reports the FILE_ATTRIBUTE_DIRECTORY bit of the target as it was
	// when the link was created. It is a hint only: the target may have changed.
	IsDir bool
}

// parseShellLink extracts the target information from the content of a .lnk file.
func parseShellLink(data []byte) (*shellLink, error) {
	if len(data) < lnkHeaderSize ||
		binary.LittleEndian.Uint32(data[0:4]) != lnkHeaderSize ||
		!bytes.Equal(data[4:20], lnkCLSID) {
		return nil, errInvalidLnk
	}
	flags := binary.LittleEndian.Uint32(data[20:24])
	attrs := binary.LittleEndian.Uint32(data[24:28])
	sl := &shellLink{IsDir: attrs&lnkFileAttributeDirectory != 0}
	off := lnkHeaderSize

	if flags&lnkHasLinkTargetIDList != 0 {
		if off+2 > len(data) {
			return nil, errInvalidLnk
		}
		off += 2 + int(binary.LittleEndian.Uint16(data[off:]))
	}

	if flags&lnkHasLinkInfo != 0 {
		// LinkInfoSize (4) + LinkInfoHeaderSize (4) + LinkInfoFlags (4)
		if off+12 > len(data) {
			return nil, errInvalidLnk
		}
		size := int(binary.LittleEndian.Uint32(data[off:]))
		infoFlags := binary.LittleEndian.Uint32(data[off+8:])
		if size < 12 {
			return nil, errInvalidLnk
		}
		if infoFlags&(lnkInfoVolumeIDAndLocalBasePath|lnkInfoCommonNetworkRelativeLinkAndPathSuffix) != 0 {
			sl.HasAbsoluteTarget = true
		}
		off += size
	}

	// StringData: NAME_STRING, RELATIVE_PATH, WORKING_DIR, COMMAND_LINE_ARGUMENTS
	// and ICON_LOCATION follow in this order, each present only if its flag is set.
	// We only need the first two.
	unicode := flags&lnkIsUnicode != 0
	readString := func() (string, error) {
		if off+2 > len(data) {
			return "", errInvalidLnk
		}
		n := int(binary.LittleEndian.Uint16(data[off:]))
		off += 2
		if !unicode {
			if off+n > len(data) {
				return "", errInvalidLnk
			}
			s := string(data[off : off+n])
			off += n
			return s, nil
		}
		if off+2*n > len(data) {
			return "", errInvalidLnk
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(data[off+2*i:])
		}
		off += 2 * n
		return string(utf16.Decode(u)), nil
	}

	if flags&lnkHasName != 0 {
		if _, err := readString(); err != nil {
			return nil, err
		}
	}
	if flags&lnkHasRelativePath != 0 {
		rel, err := readString()
		if err != nil {
			return nil, err
		}
		sl.RelativePath = rel
	}
	return sl, nil
}
