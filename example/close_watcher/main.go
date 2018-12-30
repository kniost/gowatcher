package main

import (
	"fmt"
	"github.com/kniost/gowatcher"
	"log"
	"time"
)

func main() {
	w := gowatcher.New()

	go func() {
		for {
			select {
			case event := <-w.Event:
				fmt.Println(event)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch test_folder for changes.
	if err := w.AddPath("../test_folder", false); err != nil {
		log.Fatalln(err)
	}

	// Print a list of all of the files and folders currently
	// being watched and their paths.
	for path, f := range w.RetrieveAllNodes() {
		fmt.Printf("%s: %s\n", path, f.Info.Name())
	}

	fmt.Println()

	// Close the gowatcher after gowatcher started.
	go func() {
		w.Wait()
		w.Close()
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("gowatcher closed")
}
