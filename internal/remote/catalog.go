package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const MaxPassageRunes = 240

var safeBookID = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
var gitBlobID = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Book struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	Aliases    []string `json:"aliases"`
	SourceURL  string   `json:"source_url"`
	SourceBlob string   `json:"source_git_blob_sha1"`
	StartAfter string   `json:"start_after"`
	SourcePage string   `json:"source_page"`
}

type Catalog struct {
	Books []Book
	byID  map[string]*Book
}

func ParseCatalog(raw []byte) (*Catalog, error) {
	var books []Book
	if err := json.Unmarshal(raw, &books); err != nil {
		return nil, err
	}
	if len(books) == 0 || len(books) > 30 {
		return nil, errors.New("expected 1-30 catalog books")
	}
	c := &Catalog{Books: books, byID: make(map[string]*Book)}
	for i := range books {
		b := &c.Books[i]
		if !safeBookID.MatchString(b.ID) || b.Title == "" || b.Author == "" ||
			!gitBlobID.MatchString(b.SourceBlob) || b.StartAfter == "" || c.byID[b.ID] != nil {
			return nil, fmt.Errorf("incomplete/duplicate catalog entry %q", b.ID)
		}
		u, err := url.Parse(b.SourceURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
			return nil, fmt.Errorf("invalid source URL for %q", b.ID)
		}
		c.byID[b.ID] = b
	}
	return c, nil
}

func (c *Catalog) Find(s string) *Book {
	s = normalized(s)
	for i := range c.Books {
		b := &c.Books[i]
		if s == normalized(b.ID) || s == normalized(b.Title) {
			return b
		}
		for _, alias := range b.Aliases {
			if s == normalized(alias) {
				return b
			}
		}
	}
	return nil
}

func (c *Catalog) List() string {
	var out strings.Builder
	out.WriteString("Books available:\n")
	for i, b := range c.Books {
		fmt.Fprintf(&out, "%d. %s — %s\n", i+1, b.Title, b.Author)
	}
	out.WriteString("Text READ <title>, e.g. READ NOSTROMO.")
	return out.String()
}

func (c *Catalog) FindNumber(s string) *Book {
	for i := range c.Books {
		if s == fmt.Sprint(i+1) {
			return &c.Books[i]
		}
	}
	return c.Find(s)
}

func normalized(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}
