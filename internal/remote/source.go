package remote

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
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
	loaded   map[string][]string
}

func (t *Texts) Passages(ctx context.Context, b *Book) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loaded == nil {
		t.loaded = make(map[string][]string)
	}
	if chunks := t.loaded[b.ID]; len(chunks) != 0 {
		return chunks, nil
	}
	path := filepath.Join(t.CacheDir, "books", b.ID+"-"+b.SourceBlob+".txt")
	raw, err := os.ReadFile(path)
	if err != nil || !verifyBlob(raw, b.SourceBlob) || len(raw) > maxSourceBytes {
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
		// Source parsing happens before caching, so an invalid complete text
		// is never retained merely because its source hash matches.
		chunks, err := parseText(raw, b.StartAfter)
		if err != nil {
			return nil, err
		}
		if err := atomicWrite(path, raw); err != nil {
			return nil, fmt.Errorf("caching book: %w", err)
		}
		t.loaded[b.ID] = chunks
		return chunks, nil
	}
	chunks, err := parseText(raw, b.StartAfter)
	if err != nil {
		return nil, err
	}
	t.loaded[b.ID] = chunks
	return chunks, nil
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

// parseText insists on the complete source's Gutenberg delimiters and a
// book-specific starting heading; it never sends the legal boilerplate.
func parseText(raw []byte, startAfter string) ([]string, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("source is not UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	start, end := -1, -1
	for i, line := range lines {
		s := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "\ufeff")))
		if strings.HasPrefix(s, "*** START OF") && strings.Contains(s, "PROJECT GUTENBERG EBOOK") {
			start = i + 1
		}
		if start != -1 && strings.HasPrefix(s, "*** END OF") && strings.Contains(s, "PROJECT GUTENBERG EBOOK") {
			end = i
			break
		}
	}
	if start < 0 || end <= start {
		return nil, errors.New("source lacks complete Gutenberg boundaries")
	}
	foundHeading := false
	for i := start; i < end; i++ {
		if strings.TrimSpace(lines[i]) == startAfter {
			start = i + 1
			foundHeading = true
			break
		}
	}
	if !foundHeading || start >= end {
		return nil, errors.New("source lacks starting heading")
	}
	var chunks []string
	var paragraph []string
	flush := func() {
		if len(paragraph) > 0 {
			chunks = append(chunks, splitWords(strings.Join(paragraph, " "), MaxPassageRunes)...)
			paragraph = nil
		}
	}
	for _, line := range lines[start:end] {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
		} else {
			paragraph = append(paragraph, line)
		}
	}
	flush()
	if len(chunks) == 0 {
		return nil, errors.New("source has no book passages")
	}
	return chunks, nil
}

// splitWords preserves every rune, splitting only at word boundaries unless
// a single word exceeds the limit. Each reply step is strictly bounded.
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
