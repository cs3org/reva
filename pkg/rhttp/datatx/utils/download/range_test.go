package download

import "testing"

func TestParseRange(t *testing.T) {
	testRange := "bytes=0-"
	size := int64(64)

	resRange, err := ParseRange(testRange, size)
	if err != nil {
		t.Error(err)
	}

	if len(resRange) != 1 {
		t.Error("Expected only one range to be returned")
	}

	singleResRange := resRange[0]
	if singleResRange.Start != 0 || singleResRange.Length != size {
		t.Errorf("Excpected range to start at %d and end at %d; but got %d-%d", 0, size, singleResRange.Start, singleResRange.Length)
	}

}
