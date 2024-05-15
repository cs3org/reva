package tree

import (
	"fmt"
	"strings"

	"github.com/pablodz/inotifywaitgo/inotifywaitgo"
)

type InotifyWatcher struct {
	tree *Tree
}

func NewInotifyWatcher(tree *Tree) *InotifyWatcher {
	return &InotifyWatcher{
		tree: tree,
	}
}

func (iw *InotifyWatcher) Watch(path string) {
	events := make(chan inotifywaitgo.FileEvent)
	errors := make(chan error)

	go inotifywaitgo.WatchPath(&inotifywaitgo.Settings{
		Dir:        path,
		FileEvents: events,
		ErrorChan:  errors,
		Options: &inotifywaitgo.Options{
			Recursive: true,
			Events: []inotifywaitgo.EVENT{
				inotifywaitgo.CREATE,
				inotifywaitgo.MOVED_TO,
			},
			Monitor: true,
		},
		Verbose: true,
	})

	for {
		select {
		case event := <-events:
			for _, e := range event.Events {
				if strings.HasSuffix(event.Filename, ".flock") || strings.HasSuffix(event.Filename, ".mlock") {
					continue
				}
				switch e {
				case inotifywaitgo.CREATE:
					go func() { _ = iw.tree.Scan(event.Filename, false) }()
				case inotifywaitgo.MOVED_TO:
					go func() { _ = iw.tree.Scan(event.Filename, true) }()
				}
			}

		case err := <-errors:
			if err.Error() == inotifywaitgo.NOT_INSTALLED {
				panic("Error: inotifywait is not installed")
			}
			fmt.Printf("Error: %s\n", err)
		}
	}
}
