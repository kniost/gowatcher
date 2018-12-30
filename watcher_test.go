package gowatcher

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// setup creates all required files and folders for
// the tests and returns a function that is used as
// a teardown function when the tests are done.
func setup(t testing.TB) (string, func()) {
	testDir, err := ioutil.TempDir(".", "")
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile(filepath.Join(testDir, "file.txt"),
		[]byte{}, 0755)
	if err != nil {
		t.Fatal(err)
	}

	files := []string{"file_1.txt", "file_2.txt", "file_3.txt"}

	for _, f := range files {
		filePath := filepath.Join(testDir, f)
		if err := ioutil.WriteFile(filePath, []byte{}, 0755); err != nil {
			t.Fatal(err)
		}
	}

	err = ioutil.WriteFile(filepath.Join(testDir, ".dotfile"),
		[]byte{}, 0755)
	if err != nil {
		t.Fatal(err)
	}

	testDirTwo := filepath.Join(testDir, "testDirTwo")
	err = os.Mkdir(testDirTwo, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile(filepath.Join(testDirTwo, "file_recursive.txt"),
		[]byte{}, 0755)
	if err != nil {
		t.Fatal(err)
	}

	abs, err := filepath.Abs(testDir)
	if err != nil {
		os.RemoveAll(testDir)
		t.Fatal(err)
	}
	return abs, func() {
		if os.RemoveAll(testDir); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEventString(t *testing.T) {
	e := &Event{Op: Create, Path: "/fake/path"}

	testCases := []struct {
		info     os.FileInfo
		expected string
	}{
		{nil, "???"},
		{
			&fileInfo{name: "f1", dir: true},
			"DIRECTORY \"f1\" CREATE [/fake/path]",
		},
		{
			&fileInfo{name: "f2", dir: false},
			"FILE \"f2\" CREATE [/fake/path]",
		},
	}

	for _, tc := range testCases {
		e.FileInfo = tc.info
		if e.String() != tc.expected {
			t.Errorf("expected e.String() to be %s, got %s", tc.expected, e.String())
		}
	}
}

func TestFileInfo(t *testing.T) {
	modTime := time.Now()

	fInfo := &fileInfo{
		name:    "finfo",
		size:    1,
		mode:    os.ModeDir,
		modTime: modTime,
		sys:     nil,
		dir:     true,
	}

	// Test file info methods.
	if fInfo.Name() != "finfo" {
		t.Fatalf("expected fInfo.Name() to be 'finfo', got %s", fInfo.Name())
	}
	if fInfo.IsDir() != true {
		t.Fatalf("expected fInfo.IsDir() to be true, got %t", fInfo.IsDir())
	}
	if fInfo.Size() != 1 {
		t.Fatalf("expected fInfo.Size() to be 1, got %d", fInfo.Size())
	}
	if fInfo.Sys() != nil {
		t.Fatalf("expected fInfo.Sys() to be nil, got %v", fInfo.Sys())
	}
	if fInfo.ModTime() != modTime {
		t.Fatalf("expected fInfo.ModTime() to be %v, got %v", modTime, fInfo.ModTime())
	}
	if fInfo.Mode() != os.ModeDir {
		t.Fatalf("expected fInfo.Mode() to be os.ModeDir, got %#v", fInfo.Mode())
	}

	w := New()

	w.wg.Done() // Set the waitgroup to done.

	go func() {
		// Trigger an event with the file info.
		w.TriggerEvent(Create, fInfo)
	}()

	e := <-w.Event

	if e.FileInfo != fInfo {
		t.Fatal("expected e.FileInfo to be equal to fInfo")
	}
}

func TestWatcherAdd(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()

	// Try to add a non-existing path.
	err := w.AddPath("-", false)
	if err == nil {
		t.Error("expected error to not be nil")
	}

	if err := w.AddPath(testDir, false); err != nil {
		t.Fatal(err)
	}

	files := w.RetrieveAllNodes()
	if len(files) != 7 {
		t.Errorf("expected len(nodes) to be 7, got %d", len(files))
	}

	// Make sure w.names contains testDir
	nodes := w.RetrieveAllNodes()
	if _, found := nodes[testDir]; !found {
		t.Errorf("expected fileTree to contain testDir")
	}

	if nodes[testDir].Info.Name() != filepath.Base(testDir) {
		t.Errorf("expected nodes[%q].Info.Name() to be %s, got %s",
			testDir, testDir, nodes[testDir].Info.Name())
	}

	dotFile := filepath.Join(testDir, ".dotfile")
	if _, found := nodes[dotFile]; !found {
		t.Errorf("expected to find %s", dotFile)
	}

	if nodes[dotFile].Info.Name() != ".dotfile" {
		t.Errorf("expected nodes[%q].Info.Name() to be .dotfile, got %s",
			dotFile, nodes[dotFile].Info.Name())
	}

	fileRecursive := filepath.Join(testDir, "testDirTwo", "file_recursive.txt")
	if _, found := nodes[fileRecursive]; found {
		t.Errorf("expected to not find %s", fileRecursive)
	}

	fileTxt := filepath.Join(testDir, "file.txt")
	if _, found := nodes[fileTxt]; !found {
		t.Errorf("expected to find %s", fileTxt)
	}

	if nodes[fileTxt].Info.Name() != "file.txt" {
		t.Errorf("expected nodes[%q].Info.Name() to be file.txt, got %s",
			fileTxt, nodes[fileTxt].Info.Name())
	}

	dirTwo := filepath.Join(testDir, "testDirTwo")
	if _, found := nodes[dirTwo]; !found {
		t.Errorf("expected to find %s directory", dirTwo)
	}

	if nodes[dirTwo].Info.Name() != "testDirTwo" {
		t.Errorf("expected nodes[%q].Info.Name() to be testDirTwo, got %s",
			dirTwo, nodes[dirTwo].Info.Name())
	}
}

func TestIgnore(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()

	err := w.AddPath(testDir, false)
	if err != nil {
		t.Errorf("expected error to be nil, got %s", err)
	}
	nodes := w.RetrieveAllNodes()
	if len(nodes) != 7 {
		t.Errorf("expected len(nodes) to be 7, got %d", len(nodes))
	}
	_ = w.Remove(testDir)
	err = w.IgnorePath(testDir)

	if err != nil {
		t.Errorf("expected error to be nil, got %s", err)
	}
	err = w.AddPath(testDir, false)
	nodes = w.RetrieveAllNodes()
	if len(nodes) != 0 {
		t.Errorf("expected len(nodes) to be 0, got %d", len(nodes))
	}

	// Now try to add the ignored directory.
	err = w.AddPath(testDir, false)
	if err != nil {
		t.Errorf("expected error to be nil, got %s", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected len(nodes) to be 0, got %d", len(nodes))
	}
}

func TestRemove(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()

	err := w.AddPath(testDir, false)
	if err != nil {
		t.Errorf("expected error to be nil, got %s", err)
	}
	nodes := w.RetrieveAllNodes()

	if len(nodes) != 7 {
		t.Errorf("expected len(nodes) to be 7, got %d", len(nodes))
	}

	err = w.Remove(testDir)
	nodes = w.RetrieveAllNodes()
	if err != nil {
		t.Errorf("expected error to be nil, got %s", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected len(nodes) to be 0, got %d", len(nodes))
	}

	// TODO: Test remove single file.
}

// TODO: Test remove recursive function.

func TestIgnoreHiddenFilesRecursive(t *testing.T) {
	// TODO: Write tests for ignore hidden on windows.
	if runtime.GOOS == "windows" {
		return
	}

	testDir, teardown := setup(t)
	defer teardown()

	w := New()
	w.IgnoreHiddenFiles(true)

	if err := w.AddPath(testDir, true); err != nil {
		t.Fatal(err)
	}
	nodes := w.RetrieveAllNodes()

	if len(nodes) != 7 {
		t.Errorf("expected len(nodes) to be 7, got %d", len(nodes))
	}

	if _, found := nodes[testDir]; !found {
		t.Errorf("expected to find %s", testDir)
	}

	if nodes[testDir].Info.Name() != filepath.Base(testDir) {
		t.Errorf("expected nodes[%q].Info.Name() to be %s, got %s",
			testDir, filepath.Base(testDir), nodes[testDir].Info.Name())
	}

	fileRecursive := filepath.Join(testDir, "testDirTwo", "file_recursive.txt")
	if _, found := nodes[fileRecursive]; !found {
		t.Errorf("expected to find %s", fileRecursive)
	}

	if _, found := nodes[filepath.Join(testDir, ".dotfile")]; found {
		t.Error("expected to not find .dotfile")
	}

	fileTxt := filepath.Join(testDir, "file.txt")
	if _, found := nodes[fileTxt]; !found {
		t.Errorf("expected to find %s", fileTxt)
	}

	if nodes[fileTxt].Info.Name() != "file.txt" {
		t.Errorf("expected nodes[%q].Info.Name() to be file.txt, got %s",
			fileTxt, nodes[fileTxt].Info.Name())
	}

	dirTwo := filepath.Join(testDir, "testDirTwo")
	if _, found := nodes[dirTwo]; !found {
		t.Errorf("expected to find %s directory", dirTwo)
	}

	if nodes[dirTwo].Info.Name() != "testDirTwo" {
		t.Errorf("expected nodes[%q].Info.Name() to be testDirTwo, got %s",
			dirTwo, nodes[dirTwo].Info.Name())
	}
}

func TestIgnoreHiddenFiles(t *testing.T) {
	// TODO: Write tests for ignore hidden on windows.
	if runtime.GOOS == "windows" {
		return
	}

	testDir, teardown := setup(t)
	defer teardown()

	w := New()
	w.IgnoreHiddenFiles(true)

	if err := w.AddPath(testDir, false); err != nil {
		t.Fatal(err)
	}
	nodes := w.RetrieveAllNodes()

	if len(nodes) != 6 {
		t.Errorf("expected len(nodes) to be 6, got %d", len(nodes))
	}

	if _, found := nodes[testDir]; !found {
		t.Errorf("expected to find %s", testDir)
	}

	if nodes[testDir].Info.Name() != filepath.Base(testDir) {
		t.Errorf("expected nodes[%q].Info.Name() to be %s, got %s",
			testDir, filepath.Base(testDir), nodes[testDir].Info.Name())
	}

	if _, found := nodes[filepath.Join(testDir, ".dotfile")]; found {
		t.Error("expected to not find .dotfile")
	}

	fileRecursive := filepath.Join(testDir, "testDirTwo", "file_recursive.txt")
	if _, found := nodes[fileRecursive]; found {
		t.Errorf("expected to not find %s", fileRecursive)
	}

	fileTxt := filepath.Join(testDir, "file.txt")
	if _, found := nodes[fileTxt]; !found {
		t.Errorf("expected to find %s", fileTxt)
	}

	if nodes[fileTxt].Info.Name() != "file.txt" {
		t.Errorf("expected nodes[%q].Info.Name() to be file.txt, got %s",
			fileTxt, nodes[fileTxt].Info.Name())
	}

	dirTwo := filepath.Join(testDir, "testDirTwo")
	if _, found := nodes[dirTwo]; !found {
		t.Errorf("expected to find %s directory", dirTwo)
	}

	if nodes[dirTwo].Info.Name() != "testDirTwo" {
		t.Errorf("expected nodes[%q].Info.Name() to be testDirTwo, got %s",
			dirTwo, nodes[dirTwo].Info.Name())
	}
}

func TestWatcherAddRecursive(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()

	if err := w.AddPath(testDir, true); err != nil {
		t.Fatal(err)
	}
	nodes := w.RetrieveAllNodes()

	// Make sure len(nodes) is 8.
	if len(nodes) != 8 {
		t.Errorf("expected 8 files, found %d", len(nodes))
	}

	dirTwo := filepath.Join(testDir, "testDirTwo")
	if _, found := nodes[dirTwo]; !found {
		t.Errorf("expected to find %s directory", dirTwo)
	}

	if nodes[dirTwo].Info.Name() != "testDirTwo" {
		t.Errorf("expected nodes[%q].Info.Name() to be testDirTwo, got %s",
			"testDirTwo", nodes[dirTwo].Info.Name())
	}

	fileRecursive := filepath.Join(dirTwo, "file_recursive.txt")
	if _, found := nodes[fileRecursive]; !found {
		t.Errorf("expected to find %s directory", fileRecursive)
	}

	if nodes[fileRecursive].Info.Name() != "file_recursive.txt" {
		t.Errorf("expected nodes[%q].Info.Name() to be file_recursive.txt, got %s",
			fileRecursive, nodes[fileRecursive].Info.Name())
	}
}

func TestWatcherAddNotFound(t *testing.T) {
	w := New()

	// Make sure there is an error when adding a
	// non-existent file/folder.
	if err := w.AddPath("random_filename.txt", true); err == nil {
		t.Error("expected a file not found error")
	}
}

func TestWatcherRemoveRecursive(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()

	// AddPath the testDir to the watchlist.
	if err := w.AddPath(testDir, true); err != nil {
		t.Fatal(err)
	}
	nodes := w.RetrieveAllNodes()

	// Make sure len(nodes) is 8.
	if len(nodes) != 8 {
		t.Errorf("expected 8 files, found %d", len(nodes))
	}

	// Now remove the folder from the watchlist.
	if err := w.Remove(testDir); err != nil {
		t.Error(err)
	}
	nodes = w.RetrieveAllNodes()
	// Now check that there is nothing being watched.
	if len(nodes) != 0 {
		t.Errorf("expected len(nodes) to be 0, got %d", len(nodes))
	}

	// Make sure len(w.names) is now 0.
	if len(nodes) != 0 {
		t.Errorf("expected len(w.names) to be empty, len(w.names): %d", len(nodes))
	}
}

func TestListFiles(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()
	w.AddPath(testDir, true)

	fileList := w.RetrieveAllNodes()
	if fileList == nil {
		t.Error("expected file traverseTree to not be empty")
	}

	// Make sure fInfoTest contains the correct os.FileInfo names.
	fname := filepath.Join(testDir, "file.txt")
	if fileList[fname].Info.Name() != "file.txt" {
		t.Errorf("expected fileList[%s].Name() to be file.txt, got %s",
			fname, fileList[fname].Info.Name())
	}

	// Try to call traverseTree on a file that's not a directory.
	node, _ := w.traverseTree(fname, true)
	fileList = node.RetrieveAllNodes()
	if len(fileList) != 1 {
		t.Errorf("expected len of file traverseTree to be 1, got %d", len(fileList))
	}
}

func TestTriggerEvent(t *testing.T) {
	w := New()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		select {
		case event := <-w.Event:
			if event.Name() != "triggered event" {
				t.Errorf("expected event file name to be triggered event, got %s",
					event.Name())
			}
		case <-time.After(time.Millisecond * 250):
			t.Fatal("received no event from Event channel")
		}
	}()

	go func() {
		// Start the watching process.
		if err := w.Start(time.Millisecond * 100); err != nil {
			t.Fatal(err)
		}
	}()

	w.TriggerEvent(Create, nil)

	wg.Wait()
}

func TestEventAddFile(t *testing.T) {
	testDir, teardown := setup(t)
	defer teardown()

	w := New()
	w.FilterOps(Create)

	// AddPath the testDir to the watchlist.
	if err := w.AddPath(testDir, true); err != nil {
		t.Fatal(err)
	}

	files := map[string]bool{
		"newfile_1.txt": false,
		"newfile_2.txt": false,
		"newfile_3.txt": false,
	}

	for f := range files {
		filePath := filepath.Join(testDir, f)
		if err := ioutil.WriteFile(filePath, []byte{}, 0755); err != nil {
			t.Error(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		events := 0
		for {
			select {
			case event := <-w.Event:
				if event.Op != Create {
					t.Errorf("expected event to be Create, got %s", event.Op)
				}

				files[event.Name()] = true
				events++

				if events == len(files) {
					return
				}
			case <-time.After(time.Millisecond * 250):
				for f, e := range files {
					if !e {
						t.Errorf("received no event for file %s", f)
					}
				}
				return
			}
		}
	}()

	go func() {
		// Start the watching process.
		if err := w.Start(time.Millisecond * 100); err != nil {
			t.Fatal(err)
		}
	}()

	wg.Wait()
}

// TODO: TestIgnoreFiles
func TestIgnoreFiles(t *testing.T) {}

func TestEventDeleteFile(t *testing.T) {

	testDir, teardown := setup(t)
	defer teardown()

	w := New()
	w.FilterOps(Remove)

	// AddPath the testDir to the watchlist.
	if err := w.AddPath(testDir, true); err != nil {
		t.Fatal(err)
	}

	files := map[string]bool{
		"file_1.txt": false,
		"file_2.txt": false,
		"file_3.txt": false,
	}

	for f := range files {
		filePath := filepath.Join(testDir, f)
		if err := os.Remove(filePath); err != nil {
			t.Error(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		events := 0
		for {
			select {
			case event := <-w.Event:
				if event.Op != Remove {
					t.Errorf("expected event to be Remove, got %s", event.Op)
				}

				files[event.Name()] = true
				events++

				if events == len(files) {
					return
				}
			case <-time.After(time.Millisecond * 250):
				for f, e := range files {
					if !e {
						t.Errorf("received no event for file %s", f)
					}
				}
				return
			}
		}
	}()

	go func() {
		// Start the watching process.
		if err := w.Start(time.Millisecond * 100); err != nil {
			t.Fatal(err)
		}
	}()

	wg.Wait()
}

func TestEventChmodFile(t *testing.T) {

	// Chmod is not supported under windows.
	if runtime.GOOS == "windows" {
		return
	}

	testDir, _ := setup(t)
	//defer teardown()

	w := New()
	w.FilterOps(Chmod)

	// AddPath the testDir to the watchlist.
	if err := w.AddPath(testDir, false); err != nil {
		t.Fatal(err)
	}

	files := map[string]bool{
		"file_1.txt": false,
		"file_2.txt": false,
		"file_3.txt": false,
	}

	for f := range files {
		filePath := filepath.Join(testDir, f)
		if err := os.Chmod(filePath, os.ModePerm); err != nil {
			t.Error(err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		events := 0
		for {
			select {
			case event := <-w.Event:
				if event.Op != Chmod {
					t.Errorf("expected event to be Chmod, got %s", event.Op)
				}

				files[event.Name()] = true
				events++

				if events == len(files) {
					return
				}
			case <-time.After(time.Millisecond * 250):
				for f, e := range files {
					if !e {
						t.Errorf("received no event for file %s", f)
					}
				}
				return
			}
		}
	}()

	go func() {
		// Start the watching process.
		if err := w.Start(time.Millisecond * 100); err != nil {
			t.Fatal(err)
		}
	}()

	wg.Wait()
}

func TestWatcherStartWithInvalidDuration(t *testing.T) {
	w := New()

	err := w.Start(0)
	if err != ErrDurationTooShort {
		t.Fatalf("expected ErrDurationTooShort error, got %s", err.Error())
	}
}

func TestWatcherStartWhenAlreadyRunning(t *testing.T) {
	w := New()

	go func() {
		err := w.Start(time.Millisecond * 100)
		if err != nil {
			t.Fatal(err)
		}
	}()
	w.Wait()

	err := w.Start(time.Millisecond * 100)
	if err != ErrWatcherRunning {
		t.Fatalf("expected ErrWatcherRunning error, got %s", err.Error())
	}
}

func BenchmarkListFiles(b *testing.B) {
	testDir, teardown := setup(b)
	defer teardown()

	w := New()
	err := w.AddPath(testDir, true)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		fileList := w.RetrieveAllNodes()
		if fileList == nil {
			b.Fatal("expected file traverseTree to not be empty")
		}
	}
}

func TestSetMaxEvents(t *testing.T) {
	w := New()

	if w.maxEvents != 0 {
		t.Fatalf("expected max events to be 0, got %d", w.maxEvents)
	}

	w.SetMaxEvents(3)

	if w.maxEvents != 3 {
		t.Fatalf("expected max events to be 3, got %d", w.maxEvents)
	}
}

func TestOpsString(t *testing.T) {
	testCases := []struct {
		want     Op
		expected string
	}{
		{Create, "CREATE"},
		{Write, "WRITE"},
		{Remove, "REMOVE"},
		{Chmod, "CHMOD"},
		{Op(10), "???"},
	}

	for _, tc := range testCases {
		if tc.want.String() != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.want.String())
		}
	}
}
