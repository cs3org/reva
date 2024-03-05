package tree

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type GpfsWatchFolderWatcher struct {
	tree *Tree
	c    *kafka.Consumer
}

func NewGpfsWatchFolderWatcher(tree *Tree, kafkaservers []string) (*GpfsWatchFolderWatcher, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": strings.Join(kafkaservers, ","),
		"group.id":          "ocis-posixfs",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		return nil, err
	}

	w := &GpfsWatchFolderWatcher{
		tree: tree,
		c:    c,
	}

	return w, nil
}

func (w *GpfsWatchFolderWatcher) Watch(topic string) {
	err := w.c.SubscribeTopics([]string{topic}, nil)
	if err != nil {
		return
	}

	// Set up a channel for handling Ctrl-C, etc
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	// Process messages
	run := true

	lwev := &lwe{}
	for run {
		select {
		case sig := <-sigchan:
			fmt.Printf("Caught signal %v: terminating\n", sig)
			run = false
		default:
			ev, err := w.c.ReadMessage(100 * time.Millisecond)
			if err != nil {
				// Errors are informational and automatically handled by the consumer
				continue
			}

			err = json.Unmarshal([]byte(ev.Value), lwev)
			if err != nil {
				continue
			}

			if strings.HasSuffix(lwev.Path, ".flock") || strings.HasSuffix(lwev.Path, ".mlock") {
				continue
			}

			switch {
			case strings.Contains(lwev.Event, "IN_CREATE"):
				go w.tree.Scan(lwev.Path, false)
			case strings.Contains(lwev.Event, "IN_CLOSE_WRITE"):
				bytesWritten, err := strconv.Atoi(lwev.BytesWritten)
				if err == nil && bytesWritten > 0 {
					go w.tree.Scan(lwev.Path, false)
				}
			case strings.Contains(lwev.Event, "IN_MOVED_TO"):
				go w.tree.Scan(lwev.Path, true)
			}
		}
	}

}
