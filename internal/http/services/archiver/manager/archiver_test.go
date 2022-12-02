// Copyright 2018-2022 CERN
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

package manager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	downMock "github.com/cs3org/reva/pkg/storage/utils/downloader/mock"
	walkerMock "github.com/cs3org/reva/pkg/storage/utils/walker/mock"
	"github.com/cs3org/reva/pkg/test"
)

func TestGetDeepestCommonDir(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "no paths",
			paths:    []string{},
			expected: "",
		},
		{
			name:     "one path",
			paths:    []string{"/aa/bb/cc"},
			expected: "/aa/bb/cc",
		},
		{
			name:     "root as common parent",
			paths:    []string{"/aa/bb/bb", "/bb/cc"},
			expected: "/",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc", "/aa/bb/dd"},
			expected: "/aa/bb",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc", "/aa/bb/dd", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc/", "/aa/bb/dd/", "/aa/test/"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa", "/aa/bb/dd", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa/", "/aa/bb/dd/", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa/bb/dd", "/aa/", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/reva/einstein/test", "/reva/einstein"},
			expected: "/reva/einstein",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := getDeepestCommonDir(tt.paths)
			if res != tt.expected {
				t.Fatalf("getDeepestCommondDir() failed: paths=%+v expected=%s got=%s", tt.paths, tt.expected, res)
			}
		})
	}
}

func UnTar(dir string, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // finish to read the archive
		}
		if err != nil {
			return err
		}

		p := path.Join(dir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(p, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			d := path.Dir(p)
			err := os.MkdirAll(d, 0755)
			if err != nil {
				return err
			}
			file, err := os.Create(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(file, tr)
			if err != nil {
				return err
			}
		default:
			return errors.New("not supported")
		}
	}
	return nil
}

func TestCreateTar(t *testing.T) {
	tests := []struct {
		name     string
		src      test.Dir
		config   Config
		files    []string
		expected test.Dir
		err      error
	}{
		{
			name: "one file",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     3,
				MaxNumFiles: 1,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			err: nil,
		},
		{
			name: "one big file",
			src: test.Dir{
				"foo": test.File{
					Content: strings.Repeat("a", 1024*1024),
				},
			},
			config: Config{
				MaxSize:     1024 * 1024 * 2,
				MaxNumFiles: 1000,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.File{
					Content: strings.Repeat("a", 1024*1024),
				},
			},
			err: nil,
		},
		{
			name: "one file - error max files reached",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     3,
				MaxNumFiles: 0,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxFileCount{},
		},
		{
			name: "one file - error max size reached",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     0,
				MaxNumFiles: 1,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxSize{},
		},
		{
			name: "one folder empty",
			src: test.Dir{
				"foo": test.Dir{},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 1,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.Dir{},
			},
			err: nil,
		},
		{
			name: "one folder empty - error max files reached",
			src: test.Dir{
				"foo": test.Dir{},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 0,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxFileCount{},
		},
		{
			name: "one folder - one file in",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar all",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo", "foobar", "other_dir"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar partial",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo", "foobar"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar different levels",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foobar", "other_dir/nested_dir/foo", "other_dir/nested_dir/bar"},
			expected: test.Dir{
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir with extesions",
			src: test.Dir{
				"foo": test.Dir{
					"bar.txt": test.File{
						Content: "qwerty\ntest",
					},
				},
				"main.py": test.File{
					Content: "print(\"Hello world!\")\n",
				},
				"other_dir": test.Dir{
					"images": test.Dir{
						"foo.png": test.File{
							Content: "<png content>",
						},
						"bar.jpg": test.File{
							Content: "<jpg content>",
						},
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo/bar.txt", "main.py", "other_dir"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar.txt": test.File{
						Content: "qwerty\ntest",
					},
				},
				"main.py": test.File{
					Content: "print(\"Hello world!\")\n",
				},
				"other_dir": test.Dir{
					"images": test.Dir{
						"foo.png": test.File{
							Content: "<png content>",
						},
						"bar.jpg": test.File{
							Content: "<jpg content>",
						},
					},
				},
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			tmpdir, cleanup, err := test.NewTestDir(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()

			filesAbs := []string{}
			for _, f := range tt.files {
				filesAbs = append(filesAbs, path.Join(tmpdir, f))
			}

			w := walkerMock.NewWalker()
			d := downMock.NewDownloader()

			arch, err := NewArchiver(filesAbs, w, d, tt.config)
			if err != nil {
				t.Fatal(err)
			}

			var tarFile bytes.Buffer

			err = arch.CreateTar(ctx, io.Writer(&tarFile))
			if err != tt.err {
				t.Fatalf("error result different from expected: got=%v, expected=%v", err, tt.err)
			}

			tarTmpDir, cleanup, err := test.TmpDir()
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()

			err = UnTar(tarTmpDir, &tarFile)
			if err != nil {
				t.Fatal(err)
			}

			if tt.expected != nil {
				expectedTmp, cleanup, err := test.NewTestDir(tt.expected)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				if !test.DirEquals(tarTmpDir, expectedTmp) {
					t.Fatalf("untar dir different from expected")
				}
			}
		})
	}
}

func UnZip(dir string, r io.Reader) error {
	// save the file temporarely
	tmp, cleanup, err := test.TmpDir()
	if err != nil {
		return err
	}
	defer cleanup()

	zipFile := path.Join(tmp, "tmp.zip")
	zfile, err := os.Create(zipFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(zfile, r)
	if err != nil {
		return err
	}
	zfile.Close()

	// open the tmp zip file and read it
	zr, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		p := path.Join(dir, f.Name)

		d := path.Dir(p)
		err := os.MkdirAll(d, 0755)
		if err != nil {
			return err
		}

		if strings.HasSuffix(f.Name, "/") {
			// is a dir
			err := os.Mkdir(p, 0755)
			if err != nil {
				return err
			}
		} else {
			// is a regular file
			file, err := os.Create(p)
			if err != nil {
				return err
			}

			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer zr.Close()
			_, err = io.Copy(file, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func TestCreateZip(t *testing.T) {
	tests := []struct {
		name     string
		src      test.Dir
		config   Config
		files    []string
		expected test.Dir
		err      error
	}{
		{
			name: "one file",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     3,
				MaxNumFiles: 1,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			err: nil,
		},
		{
			name: "one big file",
			src: test.Dir{
				"foo": test.File{
					Content: strings.Repeat("a", 1024*1024),
				},
			},
			config: Config{
				MaxSize:     1024 * 1024 * 2,
				MaxNumFiles: 1000,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.File{
					Content: strings.Repeat("a", 1024*1024),
				},
			},
			err: nil,
		},
		{
			name: "one file - error max files reached",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     3,
				MaxNumFiles: 0,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxFileCount{},
		},
		{
			name: "one file - error max size reached",
			src: test.Dir{
				"foo": test.File{
					Content: "foo",
				},
			},
			config: Config{
				MaxSize:     0,
				MaxNumFiles: 1,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxSize{},
		},
		{
			name: "one folder empty",
			src: test.Dir{
				"foo": test.Dir{},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 1,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.Dir{},
			},
			err: nil,
		},
		{
			name: "one folder empty - error max files reached",
			src: test.Dir{
				"foo": test.Dir{},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 0,
			},
			files:    []string{"foo"},
			expected: nil,
			err:      ErrMaxFileCount{},
		},
		{
			name: "one folder - one file in",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
			},
			config: Config{
				MaxSize:     1000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar all",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo", "foobar", "other_dir"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar partial",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo", "foobar"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir - tar different levels",
			src: test.Dir{
				"foo": test.Dir{
					"bar": test.File{
						Content: "bar",
					},
				},
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
					"nested_file": test.File{
						Content: "nested_file",
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foobar", "other_dir/nested_dir/foo", "other_dir/nested_dir/bar"},
			expected: test.Dir{
				"foobar": test.File{
					Content: "foobar",
				},
				"other_dir": test.Dir{
					"nested_dir": test.Dir{
						"foo": test.File{
							Content: "foo",
						},
						"bar": test.File{
							Content: "bar",
						},
					},
				},
			},
			err: nil,
		},
		{
			name: "multiple folders/files in root dir with extesions",
			src: test.Dir{
				"foo": test.Dir{
					"bar.txt": test.File{
						Content: "qwerty\ntest",
					},
				},
				"main.py": test.File{
					Content: "print(\"Hello world!\")\n",
				},
				"other_dir": test.Dir{
					"images": test.Dir{
						"foo.png": test.File{
							Content: "<png content>",
						},
						"bar.jpg": test.File{
							Content: "<jpg content>",
						},
					},
				},
			},
			config: Config{
				MaxSize:     100000,
				MaxNumFiles: 1000,
			},
			files: []string{"foo/bar.txt", "main.py", "other_dir"},
			expected: test.Dir{
				"foo": test.Dir{
					"bar.txt": test.File{
						Content: "qwerty\ntest",
					},
				},
				"main.py": test.File{
					Content: "print(\"Hello world!\")\n",
				},
				"other_dir": test.Dir{
					"images": test.Dir{
						"foo.png": test.File{
							Content: "<png content>",
						},
						"bar.jpg": test.File{
							Content: "<jpg content>",
						},
					},
				},
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			tmpdir, cleanup, err := test.NewTestDir(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()

			filesAbs := []string{}
			for _, f := range tt.files {
				filesAbs = append(filesAbs, path.Join(tmpdir, f))
			}

			w := walkerMock.NewWalker()
			d := downMock.NewDownloader()

			arch, err := NewArchiver(filesAbs, w, d, tt.config)
			if err != nil {
				t.Fatal(err)
			}

			var zipFile bytes.Buffer

			err = arch.CreateZip(ctx, io.Writer(&zipFile))
			if err != tt.err {
				t.Fatalf("error result different from expected: got=%v, expected=%v", err, tt.err)
			}

			if tt.expected != nil {
				zipTmpDir, cleanup, err := test.TmpDir()
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()

				err = UnZip(zipTmpDir, &zipFile)
				if err != nil {
					t.Fatal(err)
				}

				expectedTmp, cleanup, err := test.NewTestDir(tt.expected)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				if !test.DirEquals(zipTmpDir, expectedTmp) {
					t.Fatalf("unzip dir different from expected")
				}
			}
		})
	}
}
