package driver

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileSystem defines the filesystem operations required by hardware drivers.
type FileSystem interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm ...os.FileMode) error
	ReadDir(dirname string) ([]os.DirEntry, error)
	Stat(name string) (os.FileInfo, error)
	Exists(path string) bool
}

// RealFS implements FileSystem using standard os filesystem operations.
type RealFS struct{}

// NewRealFS returns a FileSystem backed by the real OS filesystem.
func NewRealFS() FileSystem {
	return &RealFS{}
}

// ReadFile reads the named file and returns its contents.
func (r *RealFS) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

// WriteFile writes data to the named file. Default permission is 0644.
func (r *RealFS) WriteFile(filename string, data []byte, perm ...os.FileMode) error {
	p := os.FileMode(0644)
	if len(perm) > 0 {
		p = perm[0]
	}
	return os.WriteFile(filename, data, p)
}

// ReadDir reads the named directory and returns a list of directory entries.
func (r *RealFS) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

// Stat returns file info for the given path.
func (r *RealFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// Exists checks if a file or directory exists.
func (r *RealFS) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MockFS is an in-memory thread-safe implementation of FileSystem for testing.
type MockFS struct {
	mu    sync.RWMutex
	files map[string][]byte
	dirs  map[string]bool
}

// NewMockFS creates an empty in-memory MockFS.
func NewMockFS() *MockFS {
	m := &MockFS{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
	m.dirs["/"] = true
	return m
}

func cleanPath(p string) string {
	return filepath.Clean(p)
}

// WriteFile stores data in the mock filesystem at the given path.
// It also automatically registers parent directories.
func (m *MockFS) WriteFile(filename string, data []byte, perm ...os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleaned := cleanPath(filename)
	// Make a copy of the data
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[cleaned] = buf

	// Mark all parent directories as existing
	dir := filepath.Dir(cleaned)
	for dir != "." && dir != "/" && dir != "" {
		m.dirs[dir] = true
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	m.dirs["/"] = true
	return nil
}

// MkdirAll creates a directory and all parent directories in the mock filesystem.
func (m *MockFS) MkdirAll(dirname string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir := cleanPath(dirname)
	for dir != "." && dir != "/" && dir != "" {
		m.dirs[dir] = true
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	m.dirs["/"] = true
}

// ReadFile reads the named file from the mock filesystem.
func (m *MockFS) ReadFile(filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleaned := cleanPath(filename)
	data, ok := m.files[cleaned]
	if !ok {
		if m.dirs[cleaned] {
			return nil, fmt.Errorf("read %s: is a directory: %w", filename, os.ErrInvalid)
		}
		return nil, fmt.Errorf("open %s: %w", filename, os.ErrNotExist)
	}

	buf := make([]byte, len(data))
	copy(buf, data)
	return buf, nil
}

// ReadDir reads directory entries in the mock filesystem.
func (m *MockFS) ReadDir(dirname string) ([]os.DirEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleaned := cleanPath(dirname)
	if !m.dirs[cleaned] {
		// Check if it exists as a file
		if _, ok := m.files[cleaned]; ok {
			return nil, fmt.Errorf("readdir %s: not a directory", dirname)
		}
		return nil, fmt.Errorf("open %s: %w", dirname, os.ErrNotExist)
	}

	entryMap := make(map[string]os.DirEntry)

	// Check direct child directories
	prefix := cleaned
	if prefix != "/" {
		prefix += "/"
	}

	for dir := range m.dirs {
		if dir == cleaned || !strings.HasPrefix(dir, prefix) {
			continue
		}
		rel := strings.TrimPrefix(dir, prefix)
		parts := strings.Split(rel, "/")
		childName := parts[0]
		if childName != "" && entryMap[childName] == nil {
			entryMap[childName] = &mockDirEntry{name: childName, isDir: true}
		}
	}

	// Check direct child files
	for f, content := range m.files {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		rel := strings.TrimPrefix(f, prefix)
		if !strings.Contains(rel, "/") && rel != "" {
			entryMap[rel] = &mockDirEntry{
				name:  rel,
				isDir: false,
				size:  int64(len(content)),
			}
		} else if strings.Contains(rel, "/") {
			parts := strings.Split(rel, "/")
			childName := parts[0]
			if childName != "" && entryMap[childName] == nil {
				entryMap[childName] = &mockDirEntry{name: childName, isDir: true}
			}
		}
	}

	var result []os.DirEntry
	for _, entry := range entryMap {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result, nil
}

// Stat returns file info for the given path in the mock filesystem.
func (m *MockFS) Stat(name string) (os.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleaned := cleanPath(name)
	if data, ok := m.files[cleaned]; ok {
		return &mockFileInfo{
			name:    filepath.Base(cleaned),
			size:    int64(len(data)),
			mode:    0644,
			modTime: time.Now(),
			isDir:   false,
		}, nil
	}

	if m.dirs[cleaned] {
		return &mockFileInfo{
			name:    filepath.Base(cleaned),
			size:    4096,
			mode:    os.ModeDir | 0755,
			modTime: time.Now(),
			isDir:   true,
		}, nil
	}

	return nil, fmt.Errorf("stat %s: %w", name, os.ErrNotExist)
}

// Exists returns true if the path exists as a file or directory.
func (m *MockFS) Exists(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleaned := cleanPath(path)
	if _, ok := m.files[cleaned]; ok {
		return true
	}
	return m.dirs[cleaned]
}

// mockDirEntry implements os.DirEntry.
type mockDirEntry struct {
	name  string
	isDir bool
	size  int64
}

func (e *mockDirEntry) Name() string               { return e.name }
func (e *mockDirEntry) IsDir() bool                { return e.isDir }
func (e *mockDirEntry) Type() fs.FileMode          { if e.isDir { return fs.ModeDir }; return 0 }
func (e *mockDirEntry) Info() (fs.FileInfo, error) {
	mode := fs.FileMode(0644)
	if e.isDir {
		mode = fs.ModeDir | 0755
	}
	return &mockFileInfo{
		name:    e.name,
		size:    e.size,
		mode:    mode,
		modTime: time.Now(),
		isDir:   e.isDir,
	}, nil
}

// mockFileInfo implements os.FileInfo.
type mockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi *mockFileInfo) Name() string       { return fi.name }
func (fi *mockFileInfo) Size() int64        { return fi.size }
func (fi *mockFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *mockFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *mockFileInfo) IsDir() bool        { return fi.isDir }
func (fi *mockFileInfo) Sys() interface{}   { return nil }
