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
		{"infierno", "A la mitad del viaje de nuestra vida"},
	} {
		b := catalog.byID[tc.id]
		doc, err := texts.load(context.Background(), b)
		if err != nil {
			t.Fatalf("live %s: %v", tc.id, err)
		}
		first, ok := doc.next(0, usual)
		if !ok || len(doc.units) < 100 || !strings.Contains(doc.body[first.Start:first.End], tc.beginning) {
			t.Fatalf("live %s wrong opening", tc.id)
		}
		if tc.id == "infierno" && doc.units[len(doc.units)-1].section != "Canto 34" {
			t.Fatal("missing final canto")
		}
	}
}
