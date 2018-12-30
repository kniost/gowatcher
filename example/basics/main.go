package main

import (
	"fmt"
	"github.com/kniost/gowatcher"
	"log"
	"time"

)

func main() {
	w := gowatcher.New()
	// Ignore hidden filess
	w.IgnoreHiddenFiles(true)
	// Uncomment to use SetMaxEvents set to 1 to allow at most 1 event to be received
	// on the Event channel per watching cycle.
	//
	// If SetMaxEvents is not set, the default is to send all events.
	w.SetMaxEvents(2)

	// Uncomment to only notify rename and move events.
	w.FilterOps(gowatcher.Create, gowatcher.Remove)

	// Uncomment to filter files based on a regular expression.
	// Only files that match the regular expression during file listing
	// will be watched.

	w.FilterName(`^abc.txt$`)

	go func() {
		for {
			select {
			case event := <-w.Event:
				fmt.Println(event) // Print the event's info.
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch this folder for changes.
	if err := w.AddPath(".", false); err != nil {
		log.Fatalln(err)
	}

	// Watch test_folder recursively for changes.
	if err := w.AddPath("../test_folder", true); err != nil {
		log.Fatalln(err)
	}

	// Print a list of all of the files and folders currently
	// being watched and their paths.
	for path, f := range w.RetrieveAllNodes() {
		fmt.Printf("%s: %s\n", path, f.Info.Name())
	}

	fmt.Println()

	// Trigger 2 events after gowatcher started.
	go func() {
		w.Wait()
		w.TriggerEvent(gowatcher.Create, nil)
		w.TriggerEvent(gowatcher.Remove, nil)
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}
}
