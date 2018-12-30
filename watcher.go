package gowatcher

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var (
	// ErrDurationTooShort occurs when calling the gowatcher's Start
	// method with a duration that's less than 1 nanosecond.
	ErrDurationTooShort = errors.New("error: duration is less than 1ns")

	// ErrWatcherRunning occurs when trying to call the gowatcher's
	// Start method and the polling cycle is still already running
	// from previously calling Start and not yet calling Close.
	ErrWatcherRunning = errors.New("error: gowatcher is already running")

	// ErrWatchedFileDeleted is an error that occurs when a file or folder that was
	// being watched has been deleted.
	ErrWatchedFileDeleted = errors.New("error: watched file or folder deleted")

	ErrWatchSymlink = errors.New("error: watch symlink")
)

// Watcher describes a process that watches files for changes.
type GoWatcher struct {
	Event  chan Event
	Error  chan error
	Closed chan struct{}
	close  chan struct{}
	wg     *sync.WaitGroup

	// mu protects the following.
	mu      *sync.RWMutex
	running bool

	fileTrees    map[string]*FileNode // map of FileNode trees, every added path will be inserted here
	nameFilters  []*regexp.Regexp
	nameIgnores  []*regexp.Regexp
	pathFilters  []*regexp.Regexp
	pathIgnores  []*regexp.Regexp
	ops          map[Op]struct{} // Op filtering.
	ignoreHidden bool            // ignore hidden files or not.
	maxEvents    int             // max sent events per cycle
}

// New creates a new Watcher.
func New() *GoWatcher {
	// Set up the WaitGroup for w.Wait().
	var wg sync.WaitGroup
	wg.Add(1)

	return &GoWatcher{
		Event:        make(chan Event),
		Error:        make(chan error),
		Closed:       make(chan struct{}),
		close:        make(chan struct{}),
		mu:           new(sync.RWMutex),
		wg:           &wg,
		fileTrees:    make(map[string]*FileNode),
		nameFilters:  make([]*regexp.Regexp, 0),
		nameIgnores:  make([]*regexp.Regexp, 0),
		pathFilters:  make([]*regexp.Regexp, 0),
		pathIgnores:  make([]*regexp.Regexp, 0),
		ignoreHidden: false,
	}
}

// SetMaxEvents controls the maximum amount of events that are sent on every Event channel per watching cycle.
// If max events is less than 1, there is no limit, which is the default.
func (w *GoWatcher) SetMaxEvents(delta int) *GoWatcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.maxEvents = delta
	return w
}

// IgnoreHiddenFiles sets the gowatcher to ignore any file or directory
// that starts with a dot.
func (w *GoWatcher) IgnoreHiddenFiles(ignore bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ignoreHidden = ignore
}

// Ignore a file or directory whose name matches the regex
func (w *GoWatcher) IgnoreName(s ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, a := range s {
		w.nameIgnores = append(w.nameIgnores, regexp.MustCompile(a))
	}
}

// The watched file or directory name must fill in one of the filters
func (w *GoWatcher) FilterName(s ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, a := range s {
		w.nameFilters = append(w.nameFilters, regexp.MustCompile(a))
	}
}

// Ignore a file or directory whose path matches the regex
func (w *GoWatcher) IgnorePath(s ...string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, a := range s {
		absPath, err := filepath.Abs(a)
		if err != nil {
			return err
		}
		re, err := regexp.Compile(absPath)
		if err != nil {
			return err
		}
		w.pathIgnores = append(w.pathIgnores, re)
	}
	return nil
}

// The watched file or directory name must fill in one of the filters
func (w *GoWatcher) FilterPath(s ...string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, a := range s {
		absPath, err := filepath.Abs(a)
		if err != nil {
			return err
		}
		re, err := regexp.Compile(absPath)
		if err != nil {
			return err
		}
		w.pathFilters = append(w.pathFilters, re)
	}
	return nil
}

func (w *GoWatcher) shouldIgnore(name string, path string) bool {
	if len(w.nameIgnores) == 0 && len(w.pathIgnores) == 0 {
		return false
	}
	for _, reg := range w.nameIgnores {
		if reg.MatchString(name) {
			return true
		}
	}
	for _, reg := range w.pathIgnores {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

func (w *GoWatcher) shouldNotice(name string, path string) bool {
	if len(w.nameFilters) == 0 && len(w.pathFilters) == 0 {
		return true
	}
	for _, reg := range w.nameFilters {
		if reg.MatchString(name) {
			return true
		}
	}
	for _, reg := range w.pathFilters {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

// FilterOps filters which event op types should be returned
// when an event occurs.
func (w *GoWatcher) FilterOps(ops ...Op) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ops = make(map[Op]struct{})
	for _, op := range ops {
		w.ops[op] = struct{}{}
	}
}

// AddPath adds either a single file or directory to the file tree.
// Parameter recursive determine whether the path be loaded recursively.
// Notice: This function should be called after ignore and filter!
func (w *GoWatcher) AddPath(path string, recursive bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		return ErrWatchSymlink
	}

	// If path is already added, just return
	if _, existed := w.fileTrees[path]; existed {
		return nil
	}

	// If hidden files are ignored and path is a hidden file or directory, simply return.
	isHidden, err := isHiddenFile(path)
	if err != nil {
		return err
	}
	if w.shouldIgnore(stat.Name(), path) || (w.ignoreHidden && isHidden) {
		return nil
	}

	// Traverse the path and its content to get a root file node.
	fileNode, err := w.traverseTree(path, recursive)
	// The outside fileNode's recursive is always true
	fileNode.recursive = true
	if err != nil {
		return err
	}

	// Add the root node to file trees.
	w.fileTrees[path] = fileNode

	return nil
}

// Generate the first added path and create a file node for every file. This function can be recursively called.
func (w *GoWatcher) traverseTree(path string, recursive bool) (node *FileNode, err error) {

	// Make sure path exists.
	stat, err := os.Lstat(path)
	if err != nil {
		return node, err
	}

	node = newNode(path, stat, recursive, w.shouldIgnore(stat.Name(), path))

	// If it's not a directory or it's ignored, just return it.
	if !stat.IsDir() || node.ignored {
		return node, nil
	}
	childMap := make(map[string]*FileNode)

	// It's a directory.
	infoList, err := ioutil.ReadDir(path)
	if err != nil {
		return node, err
	}
	// Add all of the files in the directory to the file node's children map as long
	// as they aren't on the ignored list or are hidden files if ignoreHidden
	// is set to true.
	//outer:
	for _, info := range infoList {
		name := info.Name()
		path := filepath.Join(path, name)

		isHidden, err := isHiddenFile(path)
		if err != nil {
			return node, err
		}

		shouldIgnore := w.shouldIgnore(name, path)
		if shouldIgnore || (w.ignoreHidden && isHidden) {
			continue
		}
		//fmt.Println(path)

		if !recursive {
			childMap[name] = newNode(path, info, false, shouldIgnore)
		} else if !shouldIgnore {
			childMap[name], _ = w.traverseTree(path, true)
		}

	}
	node.mu.Lock()
	defer node.mu.Unlock()
	node.Children = childMap
	return node, nil
}

func (w *GoWatcher) Remove(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, exist := w.fileTrees[path]; exist {
		delete(w.fileTrees, path)
	}
	return nil
}

//TriggerEvent is a method that can be used to trigger an event, separate to
//the file watching process.
func (w *GoWatcher) TriggerEvent(eventType Op, file os.FileInfo) {
	w.Wait()
	if file == nil {

		file = &fileInfo{name: "triggered event", modTime: time.Now()}
	}
	w.Event <- Event{Op: eventType, Path: "-", FileInfo: file}
}

// Start begins the polling cycle which repeats every specified
// duration until Close is called.
func (w *GoWatcher) Start(d time.Duration) error {
	// Return an error if d is less than 1 nanosecond.
	if d < time.Nanosecond {
		return ErrDurationTooShort
	}

	// Make sure the Watcher is not already running.
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return ErrWatcherRunning
	}
	w.running = true
	w.mu.Unlock()

	// Unblock w.Wait().
	w.wg.Done()

	for {
		// done lets the inner polling cycle loop know when the
		// current cycle's method has finished executing.
		done := make(chan struct{})

		// Any events that are found are first piped to evt before
		// being sent to the main Event channel.
		evt := make(chan Event)

		// cancel can be used to cancel the current event polling function.
		cancel := make(chan struct{})
		// Look for events.
		go func() {
			w.pollEvents(evt, cancel)
			done <- struct{}{}
		}()

		// numEvents holds the number of events for the current cycle.
		numEvents := 0

	inner:
		for {
			select {
			case <-w.close:
				close(cancel)
				close(w.Closed)
				return nil
			case event := <-evt:
				if len(w.ops) > 0 { // Filter Ops.
					_, found := w.ops[event.Op]
					if !found {
						continue
					}
				}
				if !w.shouldNotice(event.Name(), event.Path) {
					continue
				}
				numEvents++
				if w.maxEvents > 0 && numEvents > w.maxEvents {
					close(cancel)
					break inner
				}
				w.Event <- event
			case <-done: // Current cycle is finished.
				break inner
			}
		}

		// Update the file's traverseTree.
		//w.mu.Lock()
		//w.files = fileList
		//w.mu.Unlock()

		// Sleep and then continue to the next loop iteration.
		time.Sleep(d)
	}
}

func (w *GoWatcher) pollEvents(evt chan Event, cancel chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, v := range w.fileTrees {
		w.fileTrees[k] = w.pollNodeEvent(v, evt, cancel)
	}
}

// To get every node's change and generate events.
func (w *GoWatcher) pollNodeEvent(node *FileNode, evt chan Event, cancel chan struct{}) *FileNode {
	if node == nil {
		return nil
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	// If the node was ignored, don't need to check it and just return
	if node.ignored {
		return node
	}
	// Check if the path was removed
	newInfo, err := os.Lstat(node.Path)
	if err != nil {
		evt <- Event{Remove, node.Path, node.Info}
		return nil
	}
	// Compare old info and new info
	if node.Info.ModTime() != newInfo.ModTime() {
		select {
		case <-cancel:
			return node
		case evt <- Event{Write, node.Path, newInfo}:
		}
	}
	if node.Info.Mode() != newInfo.Mode() {
		select {
		case <-cancel:
			return node
		case evt <- Event{Chmod, node.Path, newInfo}:
		}
	}

	node.Info = newInfo
	// If it's not a directory or marked as non-recursive, just return.
	if !newInfo.IsDir() || !node.recursive {
		return node
	}
	// It's a directory.
	infoList, err := ioutil.ReadDir(node.Path)
	if err != nil {
		return node
	}
	// Check new file list
	infoMap := make(map[string]os.FileInfo)
	for _, info := range infoList {
		name := info.Name()
		path := filepath.Join(node.Path, name)

		isHidden, err := isHiddenFile(path)
		if err != nil {
			return node
		}

		if w.ignoreHidden && isHidden {
			continue
		}
		child, exist := node.Children[name]
		if exist {
			if child.ignored {
				continue
			}

			node.Children[name] = w.pollNodeEvent(child, evt, cancel)
		} else {
			newChild := newNode(path, info, node.recursive, w.shouldIgnore(name, path))
			node.Children[name] = newChild
			if newChild.ignored {
				continue
			}
			select {
			case <-cancel:
				return node
			case evt <- Event{Create, path, info}:
			}
			w.pollNodeEvent(newChild, evt, cancel)
		}
		infoMap[info.Name()] = info
	}
	// Examine every node.
	for k, childNode := range node.Children {
		if childNode == nil {
			//fmt.Println("find nil child node")
			delete(node.Children, k)
			continue
		}
		if _, exist := infoMap[k]; exist {
			delete(infoMap, k)
			node.Children[k] = w.pollNodeEvent(childNode, evt, cancel)
		} else {
			delete(node.Children, k)
			if childNode.ignored {
				continue
			}
			select {
			case <-cancel:
				return node
			case evt <- Event{Remove, childNode.Path, childNode.Info}:
				//fmt.Printf("Doesn't exist in infoMap : isDir: %v, pWritten: %v\n", childNode.Info.IsDir(), parentDirWritten)
			}
		}
	}
	return node
}

// Wait blocks until the gowatcher is started.
func (w *GoWatcher) Wait() {
	w.wg.Wait()
}

// Close stops a Watcher and unlocks its mutex, then sends a close signal.
func (w *GoWatcher) Close() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.fileTrees = nil
	w.mu.Unlock()
	// Send a close signal to the Start method.
	w.close <- struct{}{}
}

func (w *GoWatcher) RetrieveAllNodes() (files map[string]FileNode) {
	files = make(map[string]FileNode)
	for _, v := range w.fileTrees {
		c := v.RetrieveAllNodes()
		for k, v := range c {
			files[k] = v
		}
	}
	return files
}
