package crypto

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"hash/adler32"
	"io"
)

// ComputeMD5XS computes the MD5 checksum.
func ComputeMD5XS(r io.Reader) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ComputeAdler32XS computes the adler32 checksum.
func ComputeAdler32XS(r io.Reader) (string, error) {
	h := adler32.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ComputeSHA1XS computes the sha1 checksum.
func ComputeSHA1XS(r io.Reader) (string, error) {
	h := sha1.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
