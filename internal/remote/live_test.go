package remote

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in verification of entire HTTPS sources, not an always-on network test.
// SMS_READER_LIVE=1 go test -run TestLivePinnedBooks -v ./internal/remote
func TestLivePinnedBooks(t *testing.T) {
	if os.Getenv("SMS_READER_LIVE") != "1" {
		t.Skip("live network verification is opt-in")
	}
	raw, err := os.ReadFile("../../cmd/remote-reader/books.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	texts := &Texts{CacheDir: t.TempDir(), Client: &http.Client{Timeout: 30 * time.Second}}
	for _, tc := range []struct{ id, beginning string }{
		{"nostromo", "In the time of Spanish rule"},
		{"alice", "Alice was beginning to get very tired"},
	} {
		b := catalog.byID[tc.id]
		chunks, err := texts.Passages(context.Background(), b)
		if err != nil {
			t.Fatalf("live %s: %v", tc.id, err)
		}
		if len(chunks) < 100 || !strings.Contains(chunks[0], tc.beginning) {
			t.Fatalf("live %s wrong passages: %d, %q", tc.id, len(chunks), chunks[0])
		}
	}
}
