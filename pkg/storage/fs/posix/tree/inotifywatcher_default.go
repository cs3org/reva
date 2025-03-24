//go:build !linux

// Copyright 2025 OpenCloud GmbH <mail@opencloud.eu>
// SPDX-License-Identifier: Apache-2.0

package tree

import (
	"github.com/opencloud-eu/reva/v2/pkg/errtypes"
	"github.com/rs/zerolog"
)

// DummyWatcher is a dummy watcher that does nothing
type DummyWatcher struct{}

// Watch does nothing
func (*DummyWatcher) Watch(path string) {}

// NewInotifyWatcher returns a new inotify watcher
func NewInotifyWatcher(tree *Tree, log *zerolog.Logger) (*DummyWatcher, error) {
	return nil, errtypes.NotSupported("inotify watcher is not supported on this platform")
}
