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

package tags

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
			Alias:        "add one tag",
			InitialTags:  "",
			TagsToAdd:    "tag1",
			ExpectedTags: "tag1",
		},
		{
			Alias:        "add multiple tags",
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
		{
			Alias:        "stop adding when maximum is reached",
			InitialTags:  "a,b,c,d,e,f,g,h,i",
			TagsToAdd:    "j,k,l,m",
			ExpectedTags: "j,a,b,c,d,e,f,g,h,i",
		},
		{
			Alias:             "don't do anything when already maxed",
			InitialTags:       "a,b,c,d,e,f,g,h,i,j",
			TagsToAdd:         "k,l,m,n,o,p",
			ExpectedTags:      "a,b,c,d,e,f,g,h,i,j",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "trailing seps are ignored",
			InitialTags:  "tag1",
			TagsToAdd:    "tag2,",
			ExpectedTags: "tag2,tag1",
		},
		{
			Alias:        "special characters are allowed",
			InitialTags:  "old tag",
			TagsToAdd:    "new  tag,bettertag!",
			ExpectedTags: "new  tag,bettertag!,old tag",
		},
		{
			Alias:        "empty tags are ignored",
			InitialTags:  "tag1",
			TagsToAdd:    "tag2,,tag3",
			ExpectedTags: "tag2,tag3,tag1",
		},
		{
			Alias:             "empty tags are not ignored if there are no new tags",
			InitialTags:       "tag1",
			TagsToAdd:         ",,,tag1,,",
			ExpectedTags:      "tag1",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "condition hold for initial tags too",
			InitialTags:  "tag1,tag1,,tag3,",
			TagsToAdd:    "tag2",
			ExpectedTags: "tag2,tag1,tag3",
		},
	}

	for _, tc := range testCases {
		ts := New(tc.InitialTags)
		ts.maxtags = 10

		added := ts.Add(tc.TagsToAdd)
		require.Equal(t, tc.ExpectNoTagsAdded, !added, tc.Alias)
		require.Equal(t, tc.ExpectedTags, ts.AsList(), tc.Alias)
		require.Equal(t, strings.Split(tc.ExpectedTags, ","), ts.AsSlice(), tc.Alias)
	}

}

func TestAddTagsSlices(t *testing.T) {
	testCases := []struct {
		Alias             string
		InitialTags       []string
		TagsToAdd         []string
		ExpectedTags      string
		ExpectNoTagsAdded bool
	}{
		{
			Alias:        "add one tag",
			InitialTags:  []string{},
			TagsToAdd:    []string{"tag1"},
			ExpectedTags: "tag1",
		},
		{
			Alias:        "add multiple tags",
			InitialTags:  []string{"a", "b", "c"},
			TagsToAdd:    []string{"c,d,e"},
			ExpectedTags: "d,e,a,b,c",
		},
		{
			Alias:             "no new tags",
			InitialTags:       []string{"a", "b", "c", "d", "e", "f"},
			TagsToAdd:         []string{"c", "d", "e"},
			ExpectedTags:      "a,b,c,d,e,f",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "ignore duplicate tags",
			InitialTags:  []string{"a"},
			TagsToAdd:    []string{"b", "b", "b", "b", "a", "a", "a", "a", "a"},
			ExpectedTags: "b,a",
		},
		{
			Alias:        "stop adding when maximum is reached",
			InitialTags:  []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
			TagsToAdd:    []string{"j", "k", "l", "m"},
			ExpectedTags: "j,a,b,c,d,e,f,g,h,i",
		},
		{
			Alias:             "don't do anything when already maxed",
			InitialTags:       []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			TagsToAdd:         []string{"k", "l", "m", "n", "o", "p"},
			ExpectedTags:      "a,b,c,d,e,f,g,h,i,j",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "special characters are allowed",
			InitialTags:  []string{"old tag"},
			TagsToAdd:    []string{"new  tag", "bettertag!"},
			ExpectedTags: "new  tag,bettertag!,old tag",
		},
		{
			Alias:        "empty tags are ignored",
			InitialTags:  []string{"tag1"},
			TagsToAdd:    []string{"tag2", "", "tag3"},
			ExpectedTags: "tag2,tag3,tag1",
		},
		{
			Alias:             "empty tags are not ignored if there are no new tags",
			InitialTags:       []string{"tag1"},
			TagsToAdd:         []string{"", "", "", "tag1", "", ""},
			ExpectedTags:      "tag1",
			ExpectNoTagsAdded: true,
		},
		{
			Alias:        "condition hold for initial tags too",
			InitialTags:  []string{"tag1", "tag1", "", "tag3", ""},
			TagsToAdd:    []string{"tag2"},
			ExpectedTags: "tag2,tag1,tag3",
		},
	}

	for _, tc := range testCases {
		ts := New(tc.InitialTags...)
		ts.maxtags = 10

		added := ts.Add(tc.TagsToAdd...)
		require.Equal(t, tc.ExpectNoTagsAdded, !added, tc.Alias)
		require.Equal(t, tc.ExpectedTags, ts.AsList(), tc.Alias)
		require.Equal(t, strings.Split(tc.ExpectedTags, ","), ts.AsSlice(), tc.Alias)
	}

}

func TestRemoveTags(t *testing.T) {
	testCases := []struct {
		Alias               string
		InitialTags         string
		TagsToRemove        string
		ExpectedTags        string
		ExpectNoTagsRemoved bool
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
		{
			Alias:        "special characters are allowed",
			InitialTags:  "anothertag,btag!!,c#,distro 66",
			TagsToRemove: "distro 66,btag!!",
			ExpectedTags: "anothertag,c#",
		},
		{
			Alias:               "empty list errors",
			InitialTags:         "tag1,tag2",
			TagsToRemove:        ",,,,,",
			ExpectedTags:        "tag1,tag2",
			ExpectNoTagsRemoved: true,
		},
		{
			Alias:               "unknown tag errors",
			InitialTags:         "tag1,tag2",
			TagsToRemove:        "tag3",
			ExpectedTags:        "tag1,tag2",
			ExpectNoTagsRemoved: true,
		},
	}

	for _, tc := range testCases {
		ts := New(tc.InitialTags)

		removed := ts.Remove(tc.TagsToRemove)
		require.Equal(t, tc.ExpectNoTagsRemoved, !removed, tc.Alias)
		require.Equal(t, tc.ExpectedTags, ts.AsList(), tc.Alias)
	}

}

func TestRemoveTagsSlices(t *testing.T) {
	testCases := []struct {
		Alias               string
		InitialTags         []string
		TagsToRemove        []string
		ExpectedTags        string
		ExpectNoTagsRemoved bool
	}{
		{
			Alias:        "simple",
			InitialTags:  []string{"a", "b", "c"},
			TagsToRemove: []string{"a", "b"},
			ExpectedTags: "c",
		},
		{
			Alias:        "remove all tags",
			InitialTags:  []string{"a", "b", "c", "d", "e", "f"},
			TagsToRemove: []string{"f", "c", "a", "d", "e", "b"},
			ExpectedTags: "",
		},
		{
			Alias:        "ignore duplicate tags",
			InitialTags:  []string{"a", "b"},
			TagsToRemove: []string{"b", "b", "b", "b"},
			ExpectedTags: "a",
		},
		{
			Alias:        "order of tags is preserved",
			InitialTags:  []string{"a", "b", "c", "d"},
			TagsToRemove: []string{"a", "c"},
			ExpectedTags: "b,d",
		},
		{
			Alias:        "special characters are allowed",
			InitialTags:  []string{"anothertag", "btag!!", "c#", "distro 66"},
			TagsToRemove: []string{"distro 66", "btag!!"},
			ExpectedTags: "anothertag,c#",
		},
		{
			Alias:               "empty list errors",
			InitialTags:         []string{"tag1", "tag2"},
			TagsToRemove:        []string{"", "", "", "", "", ""},
			ExpectedTags:        "tag1,tag2",
			ExpectNoTagsRemoved: true,
		},
		{
			Alias:               "unknown tag errors",
			InitialTags:         []string{"tag1", "tag2"},
			TagsToRemove:        []string{"tag3"},
			ExpectedTags:        "tag1,tag2",
			ExpectNoTagsRemoved: true,
		},
	}

	for _, tc := range testCases {
		ts := New(tc.InitialTags...)

		removed := ts.Remove(tc.TagsToRemove...)
		require.Equal(t, tc.ExpectNoTagsRemoved, !removed, tc.Alias)
		require.Equal(t, tc.ExpectedTags, ts.AsList(), tc.Alias)
	}

}

func TestBuilders(t *testing.T) {
	testCases := []struct {
		Alias string
		List  string
		Slice []string
	}{
		{
			Alias: "simple",
			List:  "a,b,c",
			Slice: []string{"a", "b", "c"},
		},
		{
			Alias: "zero values",
			List:  "",
			Slice: nil,
		},
	}

	for _, tc := range testCases {
		list := New(tc.List)
		slice := New(tc.Slice...)

		require.Equal(t, list.AsSlice(), slice.AsSlice())
		require.Equal(t, list.AsList(), slice.AsList())
	}
}
