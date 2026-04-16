// Copyright 2018-2021 CERN
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

package mime

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		isDir    bool
		filename string
		want     string
	}{
		// Directories
		{"directory", true, "somedir", "httpd/unix-directory"},

		// Common image formats
		{"jpeg", false, "photo.jpg", "image/jpeg"},
		{"jpeg-upper", false, "PHOTO.JPG", "image/jpeg"},
		{"png", false, "image.png", "image/png"},
		{"gif", false, "anim.gif", "image/gif"},
		{"webp", false, "photo.webp", "image/webp"},
		{"tiff", false, "scan.tiff", "image/tiff"},
		{"tif", false, "scan.tif", "image/tiff"},
		{"bmp", false, "icon.bmp", "image/bmp"},
		{"svg", false, "logo.svg", "image/svg+xml"},

		// Modern image formats (added in this PR)
		{"avif", false, "photo.avif", "image/avif"},
		{"jxl", false, "photo.jxl", "image/jxl"},

		// Camera RAW formats
		{"cr2", false, "IMG_1234.cr2", "image/x-canon-cr2"},
		{"cr3", false, "IMG_1234.cr3", "image/x-canon-cr3"},
		{"nef", false, "DSC_1234.nef", "image/x-nikon-nef"},
		{"arw", false, "DSC_1234.arw", "image/x-sony-arw"},
		{"orf", false, "P1234.orf", "image/x-olympus-orf"},
		{"raf", false, "DSCF1234.raf", "image/x-fuji-raf"},
		{"rw2", false, "P1234.rw2", "image/x-panasonic-rw2"},
		{"dng", false, "photo.dng", "image/x-adobe-dng"},
		{"pef", false, "IMG_1234.pef", "image/x-pentax-pef"},
		{"heic", false, "IMG_1234.heic", "image/heic"},
		{"heif", false, "IMG_1234.heif", "image/heif"},

		// Non-image formats (sanity checks)
		{"pdf", false, "doc.pdf", "application/pdf"},
		{"mp4", false, "video.mp4", "video/mp4"},
		{"unknown", false, "file.xyz123", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.isDir, tt.filename)
			if got != tt.want {
				t.Errorf("Detect(%v, %q) = %q, want %q", tt.isDir, tt.filename, got, tt.want)
			}
		})
	}
}

func TestDetectCaseInsensitive(t *testing.T) {
	// Detect should handle uppercase extensions
	tests := []struct {
		filename string
		want     string
	}{
		{"photo.AVIF", "image/avif"},
		{"photo.Avif", "image/avif"},
		{"photo.JXL", "image/jxl"},
		{"photo.BMP", "image/bmp"},
		{"photo.CR3", "image/x-canon-cr3"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := Detect(false, tt.filename)
			if got != tt.want {
				t.Errorf("Detect(false, %q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestRegisterMime(t *testing.T) {
	// Custom MIME types should override the default map
	RegisterMime("custom123", "application/x-custom")
	got := Detect(false, "file.custom123")
	if got != "application/x-custom" {
		t.Errorf("Detect after RegisterMime = %q, want %q", got, "application/x-custom")
	}
}
