package chunking

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// assembleAll feeds every chunk of a transfer through Assemble and returns the
// result of the final call.
func assembleAll(t *testing.T, c *ChunkHandler, bodies []string) (io.ReadCloser, int64, bool) {
	t.Helper()
	var (
		r    io.ReadCloser
		size int64
		done bool
	)
	for i, body := range bodies {
		name := fmt.Sprintf("bigfile.iso-chunking-abc123-%d-%d", len(bodies), i)
		var err error
		r, size, done, err = c.Assemble(name, io.NopCloser(strings.NewReader(body)))
		if err != nil {
			t.Fatalf("chunk %d: unexpected error: %v", i, err)
		}
		if i < len(bodies)-1 && done {
			t.Fatalf("chunk %d of %d: reported done before the last chunk arrived", i, len(bodies))
		}
	}
	return r, size, done
}

func TestAssembleReportsPartialUntilTheLastChunkArrives(t *testing.T) {
	c := NewChunkHandler(t.TempDir())

	r, size, done, err := c.Assemble("bigfile.iso-chunking-abc123-3-0", io.NopCloser(strings.NewReader("aaa")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("expected done=false while two chunks are still missing")
	}
	if r != nil {
		t.Fatal("expected a nil reader for a partial transfer")
	}
	if size != 0 {
		t.Fatalf("expected size 0 for a partial transfer, got %d", size)
	}
}

// The size must describe the whole assembled file. The final request's
// Content-Length covers only its own chunk, so a caller that trusted it would
// reject every chunked upload as a short read.
func TestAssembleReturnsTheWholeFileNotTheLastChunk(t *testing.T) {
	c := NewChunkHandler(t.TempDir())
	bodies := []string{"hello ", "chunked ", "world"}

	r, size, done := assembleAll(t, c, bodies)
	if !done {
		t.Fatal("expected done=true after the final chunk")
	}
	defer r.Close()

	want := strings.Join(bodies, "")
	if size != int64(len(want)) {
		t.Fatalf("size must cover every chunk: got %d, want %d (last chunk alone is %d)",
			size, len(want), len(bodies[len(bodies)-1]))
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading the assembled file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("assembled content: got %q, want %q", got, want)
	}
}

func TestAssembledFileCloseRemovesTheTempFile(t *testing.T) {
	dir := t.TempDir()
	c := NewChunkHandler(dir)

	r, _, done := assembleAll(t, c, []string{"first", "second"})
	if !done {
		t.Fatal("expected done=true after the final chunk")
	}

	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading chunk folder: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected only the assembled temp file to remain, found %d entries", len(before))
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading chunk folder: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("Close must remove the assembled temp file, found %d entries", len(after))
	}
}
