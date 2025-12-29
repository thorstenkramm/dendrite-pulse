package files

import "sync"

// lockEntry holds a mutex and reference count for a single path.
type lockEntry struct {
	mu       sync.Mutex
	refCount int
}

// pathLocker provides per-path mutual exclusion for concurrent file operations.
// pathLocker is safe for concurrent use.
type pathLocker struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

func newPathLocker() *pathLocker {
	return &pathLocker{
		locks: make(map[string]*lockEntry),
	}
}

func (l *pathLocker) lock(path string) func() {
	l.mu.Lock()
	entry, ok := l.locks[path]
	if !ok {
		entry = &lockEntry{}
		l.locks[path] = entry
	}
	entry.refCount++
	l.mu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()

		l.mu.Lock()
		entry.refCount--
		if entry.refCount == 0 {
			delete(l.locks, path)
		}
		l.mu.Unlock()
	}
}
