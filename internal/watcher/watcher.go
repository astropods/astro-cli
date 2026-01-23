package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a directory for file changes
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	dir      string
	onChange func(path string)
	done     chan bool
}

// New creates a new file watcher
func New(dir string, onChange func(path string)) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	fw := &FileWatcher{
		watcher:  watcher,
		dir:      dir,
		onChange: onChange,
		done:     make(chan bool),
	}

	return fw, nil
}

// Start begins watching for file changes
func (fw *FileWatcher) Start() error {
	// Add the directory to watch
	if err := fw.watcher.Add(fw.dir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	// Add all subdirectories (recursive)
	if err := filepath.Walk(fw.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Only watch directories, not files
		if info == nil || !info.IsDir() {
			return nil
		}
		// Skip hidden directories and common ignore patterns
		name := filepath.Base(path)
		if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".astro" {
			return filepath.SkipDir
		}
		return fw.watcher.Add(path)
	}); err != nil {
		return fmt.Errorf("failed to watch subdirectories: %w", err)
	}

	// Start watching in background
	go fw.watch()

	return nil
}

// watch is the main event loop
func (fw *FileWatcher) watch() {
	// Debounce rapid file changes
	timer := time.NewTimer(0)
	<-timer.C // drain the timer

	var pendingPath string
	debounceDelay := 500 * time.Millisecond

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Only care about write events
			if event.Op&fsnotify.Write == fsnotify.Write {
				// Ignore certain file types
				ext := filepath.Ext(event.Name)
				if ext == ".swp" || ext == ".tmp" || ext == "~" {
					continue
				}

				// Debounce: reset timer on each event
				pendingPath = event.Name
				timer.Reset(debounceDelay)
			}

		case <-timer.C:
			// Timer expired, trigger onChange
			if pendingPath != "" {
				fw.onChange(pendingPath)
				pendingPath = ""
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("Watcher error: %v\n", err)

		case <-fw.done:
			return
		}
	}
}

// Stop stops the file watcher
func (fw *FileWatcher) Stop() {
	close(fw.done)
	fw.watcher.Close()
}
