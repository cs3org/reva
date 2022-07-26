package ocdav_test

import (
	"testing"

	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav"
	"github.com/test-go/testify/require"
)

func TestAddTags(t *testing.T) {
	testCases := []struct {
		Alias             string
		InitialTags       string
		TagsToAdd         string
		ExpectedTags      string
		ExpectNoTagsAdded bool
	}{
		{
			Alias:        "simple",
			InitialTags:  "a,b,c",
			TagsToAdd:    "c,d,e",
			ExpectedTags: "d,e,a,b,c",
		},
		{
			Alias:             "no new tags",
			InitialTags:       "a,b,c,d,e,f",
			TagsToAdd:         "c,d,e",
			ExpectedTags:      "a,b,c,d,e,f",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "ignore duplicate tags",
			InitialTags:  "a",
			TagsToAdd:    "b,b,b,b,a,a,a,a,a",
			ExpectedTags: "b,a",
		},
	}

	for _, tc := range testCases {
		tags := ocdav.FromString(tc.InitialTags)

		added := tags.AddString(tc.TagsToAdd)
		require.Equal(t, tc.ExpectNoTagsAdded, !added)
		require.Equal(t, tc.ExpectedTags, tags.AsString())
	}

}

func TestRemoveTags(t *testing.T) {
	testCases := []struct {
		Alias        string
		InitialTags  string
		TagsToRemove string
		ExpectedTags string
	}{
		{
			Alias:        "simple",
			InitialTags:  "a,b,c",
			TagsToRemove: "a,b",
			ExpectedTags: "c",
		},
		{
			Alias:        "remove all tags",
			InitialTags:  "a,b,c,d,e,f",
			TagsToRemove: "f,c,a,d,e,b",
			ExpectedTags: "",
		},
		{
			Alias:        "ignore duplicate tags",
			InitialTags:  "a,b",
			TagsToRemove: "b,b,b,b",
			ExpectedTags: "a",
		},
		{
			Alias:        "order of tags is preserved",
			InitialTags:  "a,b,c,d",
			TagsToRemove: "a,c",
			ExpectedTags: "b,d",
		},
	}

	for _, tc := range testCases {
		tags := ocdav.FromString(tc.InitialTags)

		tags.RemoveString(tc.TagsToRemove)
		require.Equal(t, tc.ExpectedTags, tags.AsString())
	}

}
