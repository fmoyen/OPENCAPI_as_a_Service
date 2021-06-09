package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/signal"
)

func newFSWatcher(files ...string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	} else {
		log.Println("watcher created. ERR=", err)
	}

	for _, f := range files {
		err = watcher.Add(f)
		if err != nil {
			log.Println("watcher.Add failure for file: ", f)
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
