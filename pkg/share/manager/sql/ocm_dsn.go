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

package sql

import (
	"errors"
	"net/url"
	"strings"
)

// ocmDatabaseConfig reserves the SQLite write lock before a transaction
// reads the share. The driver treats _timeout as an alias of _busy_timeout.
func ocmDatabaseConfig(c Config) (Config, error) {
	if c.Engine != "sqlite" {
		return c, nil
	}

	dsn := c.DBName
	if dsn == "" {
		// An empty file URI opens an unnamed temporary database and lets
		// the driver parse options after the file: prefix.
		dsn = "file:"
	} else if strings.HasPrefix(dsn, "?") {
		// The driver treats a leading question mark as a literal filename.
		dsn = "file:" + url.PathEscape(dsn)
	}
	if strings.HasPrefix(dsn, "file:") {
		dsn, _, _ = strings.Cut(dsn, "#")
	}
	path, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return c, errors.New("sqlite db_name has invalid options")
	}
	// SQLite also reads URI options directly, with different escaping
	// rules from url.ParseQuery.
	var options []string
	if rawQuery != "" {
		options = append(options, rawQuery)
	}
	for _, option := range [][2]string{{"_busy_timeout", "10000"}, {"_txlock", "immediate"}, {"_timeout", "10000"}} {
		for _, value := range q[option[0]] {
			if !strings.EqualFold(value, option[1]) {
				return c, errors.New("sqlite option " + option[0] + " must be " + option[1] + " for the OCM share manager")
			}
		}
		if option[0] != "_timeout" && !q.Has(option[0]) {
			options = append(options, option[0]+"="+option[1])
		}
	}
	c.DBName = path + "?" + strings.Join(options, "&")
	return c, nil
}
