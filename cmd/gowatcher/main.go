package main

import (
	"flag"
	"fmt"
	"github.com/kniost/gowatcher"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
	"unicode"
)

func main() {
	interval := flag.String("interval", "100ms", "gowatcher poll interval")
	recursive := flag.Bool("recursive", true, "watch folders recursively")
	dotfiles := flag.Bool("dotfiles", true, "watch dot files")
	cmd := flag.String("cmd", "", "command to run when an event occurs")
	startcmd := flag.Bool("startcmd", false, "run the command when gowatcher starts")
	listFiles := flag.Bool("list", false, "list watched files on start")
	stdinPipe := flag.Bool("pipe", false, "pipe event's info to command's stdin")
	keepalive := flag.Bool("keepalive", false, "keep alive when a cmd returns code != 0")
	ignore := flag.String("ignore", "", "comma separated list of paths to ignore")

	flag.Parse()

	// Retrieve the list of files and folders.
	files := flag.Args()

	// If no files/folders were specified, watch the current directory.
	if len(files) == 0 {
		curDir, err := os.Getwd()
		if err != nil {
			log.Fatalln(err)
		}
		files = append(files, curDir)
	}

	var cmdName string
	var cmdArgs []string
	if *cmd != "" {
		split := strings.FieldsFunc(*cmd, unicode.IsSpace)
		cmdName = split[0]
		if len(split) > 1 {
			cmdArgs = split[1:]
		}
	}

	// Create a new Watcher with the specified options.
	w := gowatcher.New()
	w.IgnoreHiddenFiles(!*dotfiles)

	// Get any of the paths to ignore.
	ignoredPaths := strings.Split(*ignore, ",")

	for _, path := range ignoredPaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}

		err := w.IgnorePath(trimmed)
		if err != nil {
			log.Fatalln(err)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		for {
			select {
			case event := <-w.Event:
				// Print the event's info.
				fmt.Println(event)

				// Run the command if one was specified.
				if *cmd != "" {
					c := exec.Command(cmdName, cmdArgs...)
					if *stdinPipe {
						c.Stdin = strings.NewReader(event.String())
					} else {
						c.Stdin = os.Stdin
					}
					c.Stdout = os.Stdout
					c.Stderr = os.Stderr
					if err := c.Run(); err != nil {
						if (c.ProcessState == nil || !c.ProcessState.Success()) && *keepalive {
							log.Println(err)
							continue
						}
						log.Fatalln(err)
					}
				}
			case err := <-w.Error:
				if err == gowatcher.ErrWatchedFileDeleted {
					fmt.Println(err)
					continue
				}
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// AddPath the files and folders specified.
	for _, file := range files {
		if *recursive {
			if err := w.AddPath(file, true); err != nil {
				log.Fatalln(err)
			}
		} else {
			if err := w.AddPath(file, false); err != nil {
				log.Fatalln(err)
			}
		}
	}

	// Print a list of all of the files and folders being watched.
	nodes := w.RetrieveAllNodes()
	if *listFiles {
		for path, f := range nodes {
			fmt.Printf("%s: %s\n", path, f.Info.Name())
		}
		fmt.Println()
	}

	fmt.Printf("Watching %d files\n", len(nodes))

	// Parse the interval string into a time.Duration.
	parsedInterval, err := time.ParseDuration(*interval)
	if err != nil {
		log.Fatalln(err)
	}

	closed := make(chan struct{})

	c := make(chan os.Signal)
	signal.Notify(c, os.Kill, os.Interrupt)
	go func() {
		<-c
		w.Close()
		<-done
		fmt.Println("gowatcher closed")
		close(closed)
	}()

	// Run the command before gowatcher starts if one was specified.
	go func() {
		if *cmd != "" && *startcmd {
			c := exec.Command(cmdName, cmdArgs...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				log.Fatalln(err)
			}
		}
	}()

	// Start the watching process.
	if err := w.Start(parsedInterval); err != nil {
		log.Fatalln(err)
	}

	<-closed
}
