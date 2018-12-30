# gowatcher


[![Build Status](https://travis-ci.org/kniost/gowatcher.svg?branch=master)](https://travis-ci.org/kniost/gowatcher)

`gowatcher` is a Go package for watching for files or directory changes (recursively or non recursively) without using filesystem events, which allows it to work cross platform consistently.

`gowatcher` watches for changes and notifies over channels either anytime an event or an error has occurred.

Events contain the `os.FileInfo` of the file or directory that the event is based on and the type of event and file or directory path.

[Installation](#installation)  
[Features](#features)  
[Example](#example)  
[Contributing](#contributing)  
[Watcher Command](#command)  

#### Chmod event is not supported under windows.

# Installation

```shell
go get -u github.com/knoist/gowatcher/...
```

# Features

- Excellent perfomance on small projects, no error even when files frequently creating and removing.
- Customizable polling interval, Event, filters and igores using regex.
- Filter Events. Events are limited to `Create`, `Remove`, `Write` and `Chmod`
- Watch folders **recursively** or non-recursively.
- Notifies the `os.FileInfo` of the file that the event is based on. e.g `Name`, `ModTime`, `IsDir`, etc.
- Notifies the full path of the file that the event is based on.
- Limit amount of events that can be received per watching cycle.
- List the files being watched.
- Trigger custom events.

# Shortcoming

If watching a large directory recursively, such as the path `/`, the CPU usage would be high.

# Example

```go
package main

import (
	"fmt"
	"github.com/kniost/gowatcher"
	"log"
	"time"

)

func main() {
	w := gowatcher.New()
	// Ignore hidden files
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

```

# Contributing
If you would ike to contribute, simply submit a pull request.

# Command

`watcher` comes with a simple command which is installed when using the `go get` command from above.

# Usage

```
Usage of watcher:
  -cmd string
    	command to run when an event occurs
  -dotfiles
    	watch dot files (default true)
  -ignore string
        comma separated list of paths to ignore
  -interval string
    	watcher poll interval (default "100ms")
  -keepalive
    	keep alive when a cmd returns code != 0
  -list
    	list watched files on start
  -pipe
    	pipe event's info to command's stdin
  -recursive
    	watch folders recursively (default true)
  -startcmd
    	run the command when watcher starts
```

All of the flags are optional and watcher can also be called by itself:
```shell
watcher
```
(watches the current directory recursively for changes and notifies any events that occur.)

A more elaborate example using the `watcher` command:
```shell
watcher -dotfiles=false -recursive=false -cmd="./myscript" main.go ../
```
In this example, `watcher` will ignore dot files and folders and won't watch any of the specified folders recursively. It will also run the script `./myscript` anytime an event occurs while watching `main.go` or any files or folders in the previous directory (`../`).

Using the `pipe` and `cmd` flags together will send the event's info to the command's stdin when changes are detected.

First create a file called `script.py` with the following contents:
```python
import sys

for line in sys.stdin:
	print (line + " - python")
```

Next, start watcher with the `pipe` and `cmd` flags enabled:
```shell
watcher -cmd="python script.py" -pipe=true
```

Now when changes are detected, the event's info will be output from the running python script.

# Thanks

Based on and inspired by the project [radovskyb/watcher](https://www.github.com/radovskyb/watcher), and change the way the watcher goes. Based on the significant changes, this project is not a fork of that library.