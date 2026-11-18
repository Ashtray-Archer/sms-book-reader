package remote

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/isomorphisms/sms-book-reader/internal/sms"
)

func gitHash(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func testService(t *testing.T) (*Service, *httptest.Server, *atomic.Int32) {
	t.Helper()
	nostromo, err := os.ReadFile("../../testdata/nostromo-excerpt.txt")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := os.ReadFile("../../testdata/alice-excerpt.txt")
	if err != nil {
		t.Fatal(err)
	}
	count := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		switch r.URL.Path {
		case "/nostromo":
			_, _ = w.Write(nostromo)
		case "/alice":
			_, _ = w.Write(alice)
		default:
			http.NotFound(w, r)
		}
	}))
	books := []Book{
		{ID: "nostromo", Title: "Nostromo", Author: "Joseph Conrad", Aliases: []string{"nostromo a tale of the seaboard"}, SourceURL: server.URL + "/nostromo", SourceBlob: gitHash(nostromo), StartAfter: "CHAPTER ONE"},
		{ID: "alice", Title: "Alice's Adventures in Wonderland", Author: "Lewis Carroll", Aliases: []string{"alice in wonderland", "alice"}, SourceURL: server.URL + "/alice", SourceBlob: gitHash(alice), StartAfter: "CHAPTER I. Down the Rabbit-Hole"},
	}
	raw, err := json.Marshal(books)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	return &Service{Catalog: catalog, Texts: &Texts{CacheDir: dir, Client: server.Client()}, Store: Store{Dir: dir}}, server, count
}

func send(t *testing.T, r *Service, from, id, body string) string {
	t.Helper()
	out, err := r.Handle(context.Background(), sms.Inbound{From: from, ID: id, Body: body})
	if err != nil {
		t.Fatalf("Handle(%q, %q): %v", id, body, err)
	}
	if len(out) != 1 || out[0].To != from {
		t.Fatalf("unexpected outgoing: %#v", out)
	}
	return out[0].Body
}

func TestRemoteFullPassagesAndDurablePositions(t *testing.T) {
	r, server, count := testService(t)
	defer server.Close()

	if list := send(t, r, "ada", "list-1", "what books do you have?"); !strings.Contains(list, "Nostromo") || !strings.Contains(list, "Alice") {
		t.Fatal(list)
	}
	first := send(t, r, "ada", "n1", "Please read Nostromo")
	if !strings.Contains(first, "In the time of Spanish rule") || strings.Contains(first, "Project Gutenberg") {
		t.Fatalf("did not deliver real literary passage: %q", first)
	}
	if got := send(t, r, "ada", "n2", "resume"); got != first {
		t.Fatalf("resume changed position: %q / %q", first, got)
	}
	second := send(t, r, "ada", "n3", "keep reading")
	if second == first || !strings.Contains(second, "ox-hides") {
		t.Fatalf("did not advance: %q", second)
	}
	if other := send(t, r, "ben", "b1", "read 1"); other != first {
		t.Fatalf("independent reader did not start at first passage: %q", other)
	}

	alice := send(t, r, "ada", "a1", "I'd like to read Alice in Wonderland")
	if !strings.Contains(alice, "Alice was beginning to get very tired") || strings.Contains(alice, "License") {
		t.Fatalf("not a real Alice passage: %q", alice)
	}
	if got := send(t, r, "ada", "n4", "Read Nostromo"); got != second {
		t.Fatalf("book-specific bookmark lost: %q", got)
	}
	if got := send(t, r, "ada", "n3", "keep reading"); got != second {
		t.Fatalf("duplicate ID changed position: %q", got)
	}
	before := count.Load()
	if before != 2 {
		t.Fatalf("did not fetch exactly one remote source per title: %d", before)
	}

	// Reconstruct the service from files; the network source is now down.
	server.Close()
	restarted := &Service{Catalog: r.Catalog, Texts: &Texts{CacheDir: r.Store.Dir, Client: r.Texts.Client}, Store: r.Store}
	if got := send(t, restarted, "ada", "n5", "pick up where I left off"); got != second {
		t.Fatalf("bookmarks/cache did not survive restart: %q", got)
	}
	if got := send(t, restarted, "ada", "n6", "next"); got == second {
		t.Fatalf("no advancement after restart: %q", got)
	}
	if count.Load() != before {
		t.Fatal("refetched source instead of using checksum-verified durable cache")
	}
	if got := send(t, restarted, "ada", "n3", "keep reading"); got != second {
		t.Fatalf("persistent idempotency receipt failed after restart: %q", got)
	}
	state, err := restarted.Store.Load("ada")
	if err != nil || state.Positions["nostromo"] < 1 || state.Positions["alice"] != 0 {
		t.Fatalf("restart positions: %+v, %v", state, err)
	}
	path := restarted.Store.path("ada")
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("state not private: %v, %v", stat, err)
	}
	if filepath.Base(path) == "ada.json" || filepath.Base(path) == "ben.json" {
		t.Fatal("raw sender leaked into filesystem path")
	}
}

func TestCannotAdvanceOnBadSource(t *testing.T) {
	r, server, _ := testService(t)
	defer server.Close()
	b := r.Catalog.byID["nostromo"]
	b.SourceBlob = strings.Repeat("0", 40)
	if _, err := r.Handle(context.Background(), sms.Inbound{From: "+15551234567", Body: "read nostromo"}); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("source identity must be checked: %v", err)
	}
	state, err := r.Store.Load("+15551234567")
	if err != nil || state.Current != "" || len(state.Positions) != 0 {
		t.Fatalf("bad source advanced cursor: %+v %v", state, err)
	}
	b.SourceBlob = gitHash([]byte("something else"))
	if _, err := r.Handle(context.Background(), sms.Inbound{From: "+15551234567", Body: "read nostromo"}); err == nil {
		t.Fatal("unexpected source accepted")
	}
}

func TestInboundDedupAndReset(t *testing.T) {
	r, server, _ := testService(t)
	defer server.Close()
	first := send(t, r, "+15551234567", "1", "start reading")
	if !strings.Contains(first, "Spanish rule") {
		t.Fatal(first)
	}
	second := send(t, r, "+15551234567", "2", "more")
	if second == first {
		t.Fatal("not advanced")
	}
	if send(t, r, "+15551234567", "2", "more") != second {
		t.Fatal("repeated inbound ID must not advance twice")
	}
	if _, err := r.Handle(context.Background(), sms.Inbound{From: "+15551234567", ID: "2", Body: "again"}); err == nil {
		t.Fatal("duplicate ID with changed content accepted")
	}
	if send(t, r, "+15551234567", "3", "start over") != first {
		t.Fatal("start over did not reset")
	}
}

func TestChunksAreBoundedAndComplete(t *testing.T) {
	longWord := strings.Repeat("界", MaxPassageRunes*2+10)
	original := "one two " + longWord + " and the end"
	chunks := splitWords(original, MaxPassageRunes)
	for _, c := range chunks {
		if utf8.RuneCountInString(c) > MaxPassageRunes {
			t.Fatalf("oversized chunk %d", utf8.RuneCountInString(c))
		}
	}
	// Split boundaries may add spaces, but may never omit book characters.
	if strings.ReplaceAll(strings.Join(chunks, ""), " ", "") != strings.ReplaceAll(original, " ", "") {
		t.Fatal("text was dropped while splitting")
	}
	if len(chunks) < 3 {
		t.Fatal("overlong word was not split")
	}
}

func TestParseRejectsMissingMarkersAndNoCache(t *testing.T) {
	bad := []byte("CHAPTER ONE\nThis is not a complete book")
	if _, err := parseText(bad, "CHAPTER ONE"); err == nil {
		t.Fatal("accepted incomplete source")
	}
	bad = []byte("*** START OF THIS PROJECT GUTENBERG EBOOK X ***\nCHAPTER ONE\nwords\n*** END OF THIS PROJECT GUTENBERG EBOOK X ***")
	if _, err := parseText(bad, "CHAPTER TWO"); err == nil {
		t.Fatal("missing start heading accepted")
	}
	// Ensure an actual excerpt's license/footer never appears in a reading chunk.
	raw, err := os.ReadFile("../../testdata/nostromo-excerpt.txt")
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := parseText(raw, "CHAPTER ONE")
	if err != nil || len(chunks) < 2 {
		t.Fatalf("parsing: %v / %d", err, len(chunks))
	}
	for _, c := range chunks {
		if strings.Contains(c, "License boilerplate") || strings.Contains(c, "Project Gutenberg") {
			t.Fatalf("header/footer leaked: %q", c)
		}
	}
}

func TestStateReplacementAndBoundedDedupe(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	st := &State{Current: "alice", Positions: map[string]int{"alice": 12}}
	if err := store.Save("reader", st); err != nil {
		t.Fatal(err)
	}
	st.Positions["alice"] = 45
	if err := store.Save("reader", st); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("reader")
	if err != nil || got.Positions["alice"] != 45 {
		t.Fatalf("atomic replacement: %+v %v", got, err)
	}
	items, err := os.ReadDir(filepath.Join(dir, "readers"))
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected temp files: %v / %v", items, err)
	}
}

func TestHTTPIngressWithToken(t *testing.T) {
	r, src, _ := testService(t)
	defer src.Close()
	h := sms.HTTPHandler(sms.Dispatch{Default: r, Services: map[string]sms.Handler{"book": r}}, "test-secret")
	server := httptest.NewServer(h)
	defer server.Close()
	post := func(body, token string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/inbound", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	payload := `{"id":"gateway-1","from":"reader","body":"book read alice"}`
	if code, _ := post(payload, "no"); code != 401 {
		t.Fatal("missing/wrong token accepted")
	}
	if code, _ := post(`{"from":"reader","body":"read alice","secret":"bad"}`, "test-secret"); code != 400 {
		t.Fatal("unknown JSON field accepted")
	}
	if code, answer := post(payload, "test-secret"); code != 200 || !strings.Contains(string(answer), "Alice was beginning") || !strings.Contains(string(answer), `"outbound"`) {
		t.Fatalf("ingress failed: HTTP %d %s", code, answer)
	}
}

func TestCorruptedCacheRefetchesPinnedText(t *testing.T) {
	r, server, count := testService(t)
	defer server.Close()
	initial := send(t, r, "ada", "1", "read nostromo")
	if count.Load() != 1 {
		t.Fatal("expected real remote request")
	}
	b := r.Catalog.byID["nostromo"]
	path := filepath.Join(r.Store.Dir, "books", b.ID+"-"+b.SourceBlob+".txt")
	if err := os.WriteFile(path, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted := &Service{Catalog: r.Catalog, Texts: &Texts{CacheDir: r.Store.Dir, Client: server.Client()}, Store: r.Store}
	if got := send(t, restarted, "ada", "2", "resume"); got != initial {
		t.Fatalf("corrupt cache not replaced by verified source: %q", got)
	}
	if count.Load() != 2 {
		t.Fatalf("expected refetch after cache corruption, got %d", count.Load())
	}
}

func TestSourceHTTPFailureDoesNotSaveCursor(t *testing.T) {
	r, server, _ := testService(t)
	server.Close()
	if _, err := r.Handle(context.Background(), sms.Inbound{From: "ada", ID: "1", Body: "read nostromo"}); err == nil || !strings.Contains(err.Error(), "bookmark unchanged") {
		t.Fatalf("missing remote source should fail without inventing text: %v", err)
	}
	st, err := r.Store.Load("ada")
	if err != nil || st.Current != "" || len(st.Positions) != 0 || len(st.Recent) != 0 {
		t.Fatalf("request failure advanced bookmark: %+v %v", st, err)
	}
}

func TestConversationalRequests(t *testing.T) {
	r, server, _ := testService(t)
	defer server.Close()
	first := send(t, r, "ada", "1", "Can I read Nostromo?")
	if !strings.Contains(first, "In the time of Spanish rule") {
		t.Fatalf("natural start request rejected: %q", first)
	}
	second := send(t, r, "ada", "2", "What happens next?")
	if second == first {
		t.Fatal("natural continuation request did not advance")
	}
	if got := send(t, r, "ada", "3", "Where did I leave off?"); got != second {
		t.Fatalf("natural resume request advanced cursor: %q", got)
	}
	if got := send(t, r, "ada", "4", "I'd like to start reading Alice in Wonderland."); !strings.Contains(got, "Alice was beginning") {
		t.Fatalf("natural book change rejected: %q", got)
	}
}
