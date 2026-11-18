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
	if !safeSender.MatchString(in.From) || len(in.Body) == 0 || len(in.Body) > 512 || len(in.ID) > 128 {
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

// answer is transport-independent. A passage is delivered on READ, not merely
// selected; RESUME/AGAIN are nonadvancing; CONTINUE advances one bounded chunk.
func (s *Service) answer(ctx context.Context, st *State, original string) (string, error) {
	cmd := normalized(strings.Trim(original, " \r\n\t.!?"))
	cmd = strings.TrimPrefix(cmd, "please ")
	cmd = strings.TrimSuffix(cmd, " please")
	switch cmd {
	case "books", "list", "catalog", "library", "show books", "show me books", "what books do you have", "what books are available":
		return s.Catalog.List(), nil
	case "help", "?":
		return "READ NOSTROMO, READ ALICE, MORE, RESUME, AGAIN, START OVER, BOOKS. Bookmarks are saved automatically.", nil
	}
	if cmd == "continue" || cmd == "continue reading" || cmd == "more" || cmd == "next" || cmd == "next passage" || cmd == "keep reading" || cmd == "go on" || cmd == "another passage" || cmd == "keep going" || cmd == "read some more" || cmd == "what happens next" || cmd == "yes" || cmd == "n" {
		return s.readChunk(ctx, st, st.Current, 1, false)
	}
	if cmd == "resume" || cmd == "resume reading" || cmd == "where was i" || cmd == "pick up where i left off" || cmd == "where did i leave off" || cmd == "resume my book" || cmd == "again" || cmd == "repeat" || cmd == "repeat that" {
		return s.readChunk(ctx, st, st.Current, 0, false)
	}
	if cmd == "start over" || cmd == "restart" || cmd == "from the beginning" {
		return s.readChunk(ctx, st, st.Current, 0, true)
	}
	if cmd == "start" || cmd == "start reading" || cmd == "begin reading" {
		book := st.Current
		if book == "" {
			book = s.Catalog.Books[0].ID
		}
		return s.readChunk(ctx, st, book, 0, false)
	}
	query := cmd
	for _, prefix := range []string{"i'd like to start reading ", "i would like to start reading ", "i would like to read ", "i'd like to read ", "i want to start reading ", "i want to read ", "can i read ", "could you read ", "can you read me ", "can you read ", "let me read ", "please read me ", "please read ", "begin reading ", "start reading ", "read me ", "read ", "begin ", "start ", "select ", "choose "} {
		if strings.HasPrefix(cmd, prefix) {
			query = strings.TrimSpace(strings.TrimPrefix(cmd, prefix))
			break
		}
	}
	query = strings.TrimPrefix(query, "the book ")
	book := s.Catalog.FindNumber(query)
	if book == nil {
		return "I didn't recognize that. Text BOOKS for titles, READ NOSTROMO, MORE, or RESUME.", nil
	}
	return s.readChunk(ctx, st, book.ID, 0, false)
}

func (s *Service) readChunk(ctx context.Context, st *State, bookID string, delta int, reset bool) (string, error) {
	if bookID == "" {
		return "Choose a book first: text BOOKS or READ NOSTROMO.", nil
	}
	b := s.Catalog.byID[bookID]
	if b == nil {
		return "This book is no longer in the catalog; text BOOKS.", nil
	}
	passages, err := s.Texts.Passages(ctx, b)
	if err != nil {
		return "", fmt.Errorf("%w; bookmark unchanged: %v", sms.ErrUpstream, err)
	}
	index, found := st.Positions[b.ID]
	if !found || reset {
		index = 0
	} else if delta != 0 {
		index += delta
	}
	if index >= len(passages) {
		return "End of " + b.Title + ". Text START OVER or READ another title.", nil
	}
	if index < 0 {
		return "", errors.New("stored reading position is invalid")
	}
	st.Current = b.ID
	st.Positions[b.ID] = index
	return fmt.Sprintf("%s %d/%d\n%s", b.Title, index+1, len(passages), passages[index]), nil
}
