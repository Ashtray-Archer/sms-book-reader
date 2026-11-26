package remote

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxSourceBytes = 4 << 20

// Texts retrieves pinned public-domain texts from a remote host, then caches
// the verified source bytes on disk for restart/offline reuse. Nothing is
// embedded as fake book content.
type Texts struct {
	Client   *http.Client
	CacheDir string
	mu       sync.Mutex
	loaded   map[string]*document
}

func (t *Texts) load(ctx context.Context, b *Book) (*document, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loaded == nil {
		t.loaded = make(map[string]*document)
	}
	key := b.ID + "-" + b.SourceBlob
	if doc := t.loaded[key]; doc != nil {
		return doc, nil
	}
	path := filepath.Join(t.CacheDir, "books", key+".txt")
	raw, err := os.ReadFile(path)
	fetched := err != nil || !verifyBlob(raw, b.SourceBlob)
	if fetched {
		client := t.Client
		if client == nil {
			client = &http.Client{Timeout: 15 * time.Second}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.SourceURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", b.ID, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("source %s returned HTTP %d", b.ID, resp.StatusCode)
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
		if err != nil || len(raw) > maxSourceBytes {
			return nil, fmt.Errorf("source %s too large/unreadable: %v", b.ID, err)
		}
		if !verifyBlob(raw, b.SourceBlob) {
			return nil, fmt.Errorf("source %s Git blob identity mismatch", b.ID)
		}
	}
	doc, err := parseDocument(raw, b)
	if err != nil {
		return nil, err
	}
	if fetched {
		if err := atomicWrite(path, raw); err != nil {
			return nil, fmt.Errorf("caching book: %w", err)
		}
	}
	t.loaded[key] = doc
	return doc, nil
}

func verifyBlob(raw []byte, expected string) bool {
	if len(raw) == 0 || len(raw) > maxSourceBytes || !gitBlobID.MatchString(expected) {
		return false
	}
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(raw))
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)) == expected
}

// Preserve the previous release's word wrapping only to locate saved pages.
func splitWords(paragraph string, maxRunes int) []string {
	var chunks []string
	var line string
	for _, word := range strings.Fields(paragraph) {
		for utf8.RuneCountInString(word) > maxRunes {
			if line != "" {
				chunks = append(chunks, line)
				line = ""
			}
			breakAt := 0
			for at := range word {
				if utf8.RuneCountInString(word[:at]) == maxRunes {
					breakAt = at
					break
				}
			}
			if breakAt == 0 { // a single rune could occupy several bytes
				breakAt = len(word)
			}
			chunks = append(chunks, word[:breakAt])
			word = word[breakAt:]
		}
		if line == "" {
			line = word
		} else if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= maxRunes {
			line += " " + word
		} else {
			chunks = append(chunks, line)
			line = word
		}
	}
	if line != "" {
		chunks = append(chunks, line)
	}
	return chunks
}
