package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/isomorphisms/sms-book-reader/internal/sms"
)

var safeSender = regexp.MustCompile(`^\+?[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

type Service struct {
	Catalog *Catalog
	Texts   *Texts
	Store   Store
	mu      sync.Mutex // single-process cursor transitions; only one instance per state dir
}

func (s *Service) Handle(ctx context.Context, in sms.Inbound) ([]sms.Outbound, error) {
	if !safeSender.MatchString(in.From) || len(in.Body) > 512 || len(in.ID) > 128 {
		return nil, errors.New("invalid sender, message, or inbound ID")
	}
	if s.Catalog == nil || s.Texts == nil {
		return nil, errors.New("reader not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.Store.Load(in.From)
	if err != nil {
		return nil, err
	}
	bodyHashBytes := sha256.Sum256([]byte(in.Body))
	bodyHash := hex.EncodeToString(bodyHashBytes[:])
	if in.ID != "" {
		for _, receipt := range st.Recent {
			if receipt.ID == in.ID {
				if receipt.BodyHash != bodyHash {
					return nil, errors.New("inbound ID reused with different body")
				}
				return []sms.Outbound{{To: in.From, Body: receipt.Reply}}, nil
			}
		}
	}
	answer, err := s.answer(ctx, st, in.Body)
	if err != nil {
		return nil, err // do not advance if remote retrieval or storage fails
	}
	if in.ID != "" {
		st.Recent = append(st.Recent, Receipt{ID: in.ID, BodyHash: bodyHash, Reply: answer})
		if len(st.Recent) > maxReceipts {
			st.Recent = st.Recent[len(st.Recent)-maxReceipts:]
		}
	}
	if err := s.Store.Save(in.From, st); err != nil {
		return nil, err
	}
	return []sms.Outbound{{To: in.From, Body: answer}}, nil
}

// The shelf and the conversation use the same handler for HTTP and the CLI.
// Digits select a book from anywhere. Zero turns a page without typing a title.
func (s *Service) answer(ctx context.Context, st *State, original string) (string, error) {
	raw := normalized(original)
	cmd := strings.Trim(strings.TrimSpace(raw), " \t.!?¿¡")
	cmd = strings.TrimPrefix(cmd, "please ")
	cmd = strings.TrimSuffix(cmd, " please")
	if raw == "?" || cmd == "help" || cmd == "ayuda" {
		if st.Current == "" {
			return s.Catalog.List(), nil
		}
		return help(st.Current), nil
	}
	switch cmd {
	case "", "book", "books", "libros", "list", "catalog", "library", "#", "show books", "show me books", "what books do you have", "what books are available":
		return s.Catalog.List(), nil
	case "hi", "hello", "hola", "read", "leer", "start", "start reading", "begin reading", "i want to read":
		return s.turn(ctx, st, st.Current, repeat, usual, "")
	case "0", "n", "next", "more", "siguiente", "sigue", "otro", "continue", "continue reading", "next passage", "keep reading", "go on", "another passage", "keep going", "read some more", "what happens next", "yes":
		return s.turn(ctx, st, st.Current, advance, usual, "")
	case "longer", "more text", "más largo", "mas largo", "un poco más largo", "un poco mas largo":
		return s.turn(ctx, st, st.Current, advance, long, "")
	case "shorter", "less text", "más corto", "mas corto", "menos":
		return s.turn(ctx, st, st.Current, advance, short, "")
	case "00", "just the text", "solo el texto", "sólo el texto", "again", "otra vez", "repite", "resume", "resume reading", "where was i", "pick up where i left off", "where did i leave off", "resume my book", "repeat", "repeat that":
		return s.turn(ctx, st, st.Current, repeat, usual, "")
	case "b", "back", "previous", "atrás", "atras", "anterior":
		return s.turn(ctx, st, st.Current, previous, usual, "")
	case "start over", "restart", "from the beginning", "desde el principio":
		return s.turn(ctx, st, st.Current, restart, usual, "")
	case "where", "dónde", "donde", "lugar":
		return s.turn(ctx, st, st.Current, location, usual, "")
	case "otra", "otra versión", "otra version", "versión de trabajo", "version de trabajo", "compare", "compare translations":
		return s.turn(ctx, st, st.Current, workingVersion, usual, "")
	case "it", "italian", "italiano", "original", "in italian", "en italiano":
		return s.turn(ctx, st, st.Current, italian, usual, "")
	case "*", "what does that mean", "what does this mean", "qué significa eso", "qué significa esto", "que significa eso", "que significa esto", "note", "notes", "nota", "notas", "explain", "por qué", "why":
		return s.turn(ctx, st, st.Current, annotation, usual, "")
	case "pause", "pausa":
		if st.Current == "" {
			return s.Catalog.List(), nil
		}
		return "Place saved. 00 rereads this passage. 0 continues.", nil
	}
	query := cmd
	for _, prefix := range []string{"i'd like to start reading ", "i would like to start reading ", "i would like to read ", "i'd like to read ", "i want to start reading ", "i want to read ", "can i read ", "could you read ", "can you read me ", "can you read ", "let me read ", "please read me ", "please read ", "begin reading ", "start reading ", "read me ", "read ", "begin ", "start ", "select ", "choose "} {
		if strings.HasPrefix(query, prefix) {
			query = strings.TrimSpace(strings.TrimPrefix(query, prefix))
			break
		}
	}
	if book := s.Catalog.FindNumber(strings.TrimPrefix(query, "the book ")); book != nil {
		return s.turn(ctx, st, book.ID, repeat, usual, "")
	}
	if st.Current == "" {
		return s.Catalog.List(), nil
	}
	if strings.HasSuffix(raw, "?") {
		return s.turn(ctx, st, st.Current, annotation, usual, cmd)
	}
	for _, prefix := range []string{"?", "¿", "note ", "nota ", "why ", "what ", "qué ", "que ", "por qué ", "por que "} {
		if strings.HasPrefix(raw, prefix) {
			return s.turn(ctx, st, st.Current, annotation, usual, cmd)
		}
	}
	return help(st.Current), nil
}

func help(current string) string {
	keys := "0 next, 00 reread, B back, # books, * notes, WHERE for your place. LONGER or SHORTER changes the next passage. Ask about a word with ?."
	if current == "infierno" {
		keys += " IT for Italian. OTRA for the working Spanish."
	}
	return keys
}

type action int

const (
	repeat action = iota
	advance
	previous
	restart
	location
	italian
	workingVersion
	annotation
)

func (s *Service) turn(ctx context.Context, st *State, bookID string, act action, size pace, query string) (string, error) {
	b := s.Catalog.byID[bookID]
	if b == nil {
		return s.Catalog.List(), nil
	}
	doc, err := s.Texts.load(ctx, b)
	if err != nil {
		return "", fmt.Errorf("%w; bookmark unchanged: %v", sms.ErrUpstream, err)
	}
	mark, found := st.Bookmarks[b.ID]
	if act == restart {
		found = false
	} else if found {
		if mark.Source != b.SourceBlob || !doc.valid(mark.Page) {
			return "", errors.New("saved place does not match this edition")
		}
	} else if old, ok := st.Positions[b.ID]; ok {
		mark.Page, found = doc.oldPage(old)
		if !found {
			return "", errors.New("invalid bookmark from the previous reader")
		}
	}
	if !found {
		first, _ := doc.next(0, size) // parseDocument requires a nonempty edition
		mark = bookmark{Page: first}
	} else {
		switch act {
		case advance:
			next, ok := doc.next(mark.Page.End, size)
			if !ok {
				return "End of " + b.Title + ". 00 rereads the ending. # opens your books.", nil
			}
			mark.Trail = append(mark.Trail, mark.Page)
			if len(mark.Trail) > 20 {
				mark.Trail = mark.Trail[len(mark.Trail)-20:]
			}
			mark.Page = next
		case previous:
			if n := len(mark.Trail); n > 0 {
				mark.Page = mark.Trail[n-1]
				if !doc.valid(mark.Page) {
					return "", errors.New("invalid saved previous page")
				}
				mark.Trail = mark.Trail[:n-1]
			} else if p, ok := doc.before(mark.Page.Start); ok {
				mark.Page = p
			}
		}
	}
	mark.Source = b.SourceBlob
	st.Current = b.ID
	st.Bookmarks[b.ID] = mark
	delete(st.Positions, b.ID)
	switch act {
	case location:
		return doc.place(b, mark.Page), nil
	case italian, workingVersion, annotation:
		return doc.studyReply(mark.Page, act, query), nil
	default:
		return strings.TrimSpace(doc.body[mark.Page.Start:mark.Page.End]), nil
	}
}
