package gowatcher

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// An Op is a type that is used to describe what type
// of event has occurred during the watching process.
type Op uint8

// Ops
const (
	Create Op = iota
	Write
	Remove
	Chmod
	//Rename
	//Move
)

var ops = map[Op]string{
	Create: "CREATE",
	Write:  "WRITE",
	Remove: "REMOVE",
	Chmod:  "CHMOD",
	//Rename: "RENAME",
	//Move:   "MOVE",
}

// String prints the string version of the Op consts
func (e Op) String() string {
	if op, found := ops[e]; found {
		return op
	}
	return "???"
}

// An Event describes an event that is received when files or directory
// changes occur. It includes the os.FileInfo of the changed file or
// directory and the type of event that's occurred and the full path of the file.
type Event struct {
	Op
	Path string
	os.FileInfo
}

// String returns a string depending on what type of event occurred and the
// file name associated with the event.
func (e Event) String() string {
	if e.FileInfo == nil {
		return "???"
	}

	pathType := "FILE"
	if e.IsDir() {
		pathType = "DIRECTORY"
	}
	return fmt.Sprintf("%s %q %s [%s]", pathType, e.Name(), e.Op, e.Path)
}

/**
Using a Trie tree data structure to improve the refresh and poll event performance
*/
type FileNode struct {
	Path      string      // Full path
	Info      os.FileInfo // File info
	ignored   bool        // Whether this FileNode ignored. If ignored, gowatcher won't try to find its children
	recursive bool        // Whether this FileNode should be recursively traversed
	mu        *sync.RWMutex
	Children  map[string]*FileNode // Children nodes, use filename as key
}

func newNode(path string, info os.FileInfo, recursive bool, ignored bool) *FileNode {
	return &FileNode{
		Path:      path,
		Info:      info,
		mu:        new(sync.RWMutex),
		recursive: recursive,
		Children:  make(map[string]*FileNode),
		ignored:   ignored || info.Mode()&os.ModeSymlink != 0,
	}
}

func (node *FileNode) String() string {
	return node.Path + strconv.FormatBool(node.ignored)
}

func (node *FileNode) RetrieveAllNodes() (files map[string]FileNode) {
	files = make(map[string]FileNode)
	node.retrieveAllNodes(files)
	return files
}

func (node *FileNode) retrieveAllNodes(files map[string]FileNode) {
	files[node.Path] = *node
	for _, v := range node.Children {
		v.retrieveAllNodes(files)
	}
}

// fileInfo is an implementation of os.FileInfo that can be used
// as a mocked os.FileInfo when triggering an event when the specified
// os.FileInfo is nil.
type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	sys     interface{}
	dir     bool
}

func (fs *fileInfo) IsDir() bool {
	return fs.dir
}
func (fs *fileInfo) ModTime() time.Time {
	return fs.modTime
}
func (fs *fileInfo) Mode() os.FileMode {
	return fs.mode
}
func (fs *fileInfo) Name() string {
	return fs.name
}
func (fs *fileInfo) Size() int64 {
	return fs.size
}
func (fs *fileInfo) Sys() interface{} {
	return fs.sys
}
