package main

import (
	log "github.com/Sirupsen/logrus" // OCP-CAPI-changes
	"github.com/fsnotify/fsnotify"
	"os"
	"os/signal"
)

func newFSWatcher(files ...string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	// OCP-CAPI-changes
	} else {
		log.Println("watcher created. ERR=", err)
	// end of OCP-CAPI-changes
	}

	for _, f := range files {
		err = watcher.Add(f)
		if err != nil {
			log.Println("watcher.Add failure for file: ", f) // OCP-CAPI-changes
			watcher.Close()
			return nil, err
		}
	}

	return watcher, nil
}

func newOSWatcher(sigs ...os.Signal) chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, sigs...)

	return sigChan
}
