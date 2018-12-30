package main

import (
	"fmt"
	"github.com/kniost/gowatcher"
	"log"
	"time"

)

func main() {
	w := gowatcher.New()

	// Ignore hidden files.
	w.IgnoreHiddenFiles(true)

	go func() {
		for {
			select {
			case event := <-w.Event:
				// Print the event's info.
				fmt.Println(event)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch test_folder recursively for changes.
	//
	// Watcher won't add .dotfile to the watchlist.
	if err := w.AddPath("../test_folder", true); err != nil {
		log.Fatalln(err)
	}

	// Print a list of all of the files and folders currently
	// being watched and their paths.
	for path, f := range w.RetrieveAllNodes() {
		fmt.Printf("%s: %s\n", path, f.Info.Name())
	}

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}
}
