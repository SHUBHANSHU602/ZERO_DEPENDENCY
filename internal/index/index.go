// Package index provides indexing for pulselog.
package index

import "sync"

// Entry identifies one record in a WAL segment.
type Entry struct {
	SegmentID uint32
	Offset    int64
	Length    uint32
}

// Index is a concurrency-safe mapping from keys to their latest WAL records.
type Index struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// New returns an empty index.
func New() *Index { return &Index{entries: make(map[string]Entry)} }

// Set records the current location of key.
func (idx *Index) Set(key string, entry Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.entries == nil {
		idx.entries = make(map[string]Entry)
	}
	idx.entries[key] = entry
}

// Get returns the current location of key.
func (idx *Index) Get(key string) (Entry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	entry, ok := idx.entries[key]
	return entry, ok
}

// Delete removes key from the index.
func (idx *Index) Delete(key string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.entries, key)
}
