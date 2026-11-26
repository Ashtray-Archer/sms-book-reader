package remote

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type unit struct {
	span
	section    string
	standalone bool // a stanza or an explicitly aligned study passage
	study      *comparison
}

type document struct {
	body      string
	units     []unit
	legacy    []span // old 240-character page indices, including chapter headings
	footnotes map[string]string
}

type pace int

const (
	usual pace = iota
	short
	long
)

// Each range points into the same text. A change of pace starts at the first
// unread word, even if the sender's previous bookmark used the old word wrap.
func (d *document) next(after int, size pace) (span, bool) {
	i := sort.Search(len(d.units), func(i int) bool { return d.units[i].End > after })
	if i == len(d.units) {
		return span{}, false
	}
	first := d.units[i]
	page := span{max(first.Start, after), first.End}
	target := 180
	if size == long {
		target = 480
	}
	for j := i + 1; size != short && j < len(d.units); j++ {
		next := d.units[j]
		if next.section != first.section || (first.study == nil) != (next.study == nil) || utf8.RuneCountInString(d.body[page.Start:next.End]) > target {
			break
		}
		if size == usual && (first.standalone || next.standalone || strings.Contains(d.body[page.End:next.Start], "\n\n")) {
			break
		}
		page.End = next.End
	}
	return page, true
}

func (d *document) before(at int) (span, bool) {
	i := sort.Search(len(d.units), func(i int) bool { return d.units[i].Start >= at }) - 1
	if i < 0 {
		return span{}, false
	}
	u := d.units[i]
	return span{u.Start, min(at, u.End)}, true
}

func (d *document) valid(p span) bool {
	return p.Start >= 0 && p.Start < p.End && p.End <= len(d.body) &&
		utf8.RuneStart(d.body[p.Start]) && (p.End == len(d.body) || utf8.RuneStart(d.body[p.End])) && strings.TrimSpace(d.body[p.Start:p.End]) != ""
}

func (d *document) onPage(p span) []unit {
	start := sort.Search(len(d.units), func(i int) bool { return d.units[i].End > p.Start })
	end := sort.Search(len(d.units), func(i int) bool { return d.units[i].Start >= p.End })
	return d.units[start:end]
}

func (d *document) oldPage(index int) (span, bool) {
	if index < 0 || index >= len(d.legacy) {
		return span{}, false
	}
	p := d.legacy[index]
	if p.Start == p.End { // the previous display contained only a heading or footer
		if next, ok := d.next(p.End, usual); ok {
			return next, true
		}
		return d.before(len(d.body))
	}
	return p, true
}

var noteID = regexp.MustCompile("^[[]([0-9]+)[]] ")
var noteRef = regexp.MustCompile("[[]([0-9]+)[]]")

func parseDocument(raw []byte, book *Book) (*document, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("source is not UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	aranda := book.Format == "aranda-infierno"
	if !aranda {
		var err error
		lines, err = gutenbergLines(lines)
		if err != nil {
			return nil, err
		}
	}
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == book.StartAfter {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, errors.New("source lacks starting heading")
	}

	doc := &document{footnotes: make(map[string]string)}
	var body strings.Builder
	section := book.StartAfter
	canto := 1
	if aranda {
		section = "Canto 1"
	}
	for i := start; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		var rows []string
		indented := true
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			indented = indented && strings.HasPrefix(lines[i], "    ")
			rows = append(rows, strings.Join(strings.Fields(lines[i]), " "))
			i++
		}
		paragraph := strings.Join(rows, " ")
		legacyParagraph := paragraph
		removed := 0
		if aranda {
			switch {
			case paragraph == "[Ilustración]":
				continue
			case strings.HasPrefix(paragraph, "_CANTO ") && strings.HasSuffix(paragraph, "_"):
				canto++
				section = fmt.Sprintf("Canto %d", canto)
				continue
			}
			if m := noteID.FindStringSubmatch(paragraph); m != nil {
				doc.footnotes[m[1]] = strings.TrimSpace(strings.TrimPrefix(paragraph, m[0]))
				continue
			}
		} else {
			// A heading can share a paragraph with the following sentence.
			header := rows[0]
			if strings.HasPrefix(header, "CHAPTER ") || strings.HasPrefix(header, "PART ") {
				section = header
				rows = rows[1:]
				paragraph = strings.Join(rows, " ")
				removed = len(legacyParagraph) - len(paragraph)
			}
			if paragraph == "" || paragraph == "THE END" || strings.HasPrefix(paragraph, "End of Project Gutenberg") {
				doc.addLegacy(legacyParagraph, body.Len(), len(legacyParagraph))
				continue
			}
		}

		verse := !aranda && indented && len(rows) >= 2
		if verse {
			paragraph = strings.Join(rows, "\n")
		}
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		offset := body.Len()
		body.WriteString(paragraph)
		if !aranda {
			doc.addLegacy(legacyParagraph, offset, removed)
		}
		var units []unit
		if aranda && len(doc.units) == 0 {
			rest := paragraph
			for i := range infiernoOpening {
				c := &infiernoOpening[i]
				if !strings.HasPrefix(rest, c.spanish) {
					return nil, errors.New("Infierno opening differs from the aligned edition")
				}
				from := len(paragraph) - len(rest)
				units = append(units, unit{span: span{from, from + len(c.spanish)}, standalone: true, study: c})
				rest = strings.TrimLeft(rest[len(c.spanish):], " ")
			}
			units = append(units, literaryUnits(rest, len(paragraph)-len(rest), false)...)
		} else {
			units = literaryUnits(paragraph, 0, verse)
		}
		for _, u := range units {
			u.Start += offset
			u.End += offset
			u.section = section
			doc.units = append(doc.units, u)
		}
	}
	if len(doc.units) == 0 {
		return nil, errors.New("source has no book passages")
	}
	doc.body = body.String()
	return doc, nil
}

// Map old folded pages back to their words. A verse newline occupies the
// same byte as a folded space. A suppressed heading maps to the next passage.
func (d *document) addLegacy(paragraph string, offset, removed int) {
	start := 0
	for _, old := range splitWords(paragraph, MaxPassageRunes) {
		for start < len(paragraph) && paragraph[start] == ' ' {
			start++
		}
		end := start + len(old)
		d.legacy = append(d.legacy, span{offset + max(0, start-removed), offset + max(0, end-removed)})
		start = end
	}
}

func gutenbergLines(lines []string) ([]string, error) {
	start := -1
	for i, line := range lines {
		s := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "\ufeff")))
		if !strings.Contains(s, "PROJECT GUTENBERG EBOOK") {
			continue
		}
		if strings.HasPrefix(s, "*** START OF") {
			start = i + 1
		}
		if start >= 0 && i > start && strings.HasPrefix(s, "*** END OF") {
			return lines[start:i], nil
		}
	}
	return nil, errors.New("source lacks complete Gutenberg boundaries")
}

// Prose turns at a sentence end. An unusually long sentence can turn at a
// clause; whitespace is the last resort. Keep closing quotation marks with
// their sentence and keep verse line breaks inside a stanza.
func literaryUnits(text string, offset int, verse bool) []unit {
	const ceiling = 360
	var units []unit
	for len(text) != 0 {
		start := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
		text, offset = text[start:], offset+start
		if len(text) == 0 {
			break
		}
		count, end, lastSpace, lastClause, lastLine := 0, 0, 0, 0, 0
		for i, r := range text {
			if count == ceiling {
				for _, cut := range []int{lastLine, lastClause, lastSpace, i} {
					if cut > 0 {
						end = cut
						break
					}
				}
				break
			}
			count++
			if unicode.IsSpace(r) {
				lastSpace = i
				if r == '\n' {
					lastLine = i
				}
				if !verse {
					word := text[:i]
					if endsSentence(word) {
						end = i
						break
					}
					switch word[len(word)-1] {
					case ',', ';', ':':
						lastClause = i
					}
				}
			}
		}
		if end == 0 {
			end = len(text)
		}
		units = append(units, unit{span: span{offset, offset + end}, standalone: verse})
		text, offset = text[end:], offset+end
	}
	return units
}

func endsSentence(prefix string) bool {
	word := prefix[strings.LastIndexByte(prefix, ' ')+1:]
	word = strings.TrimRight(word, "\"'”’»)]")
	if word == "" {
		return false
	}
	switch word[len(word)-1] {
	case '!', '?':
		return true
	case '.':
		switch strings.ToLower(word) {
		case "mr.", "mrs.", "ms.", "dr.", "st.", "sr.", "sra.", "d.", "e.g.", "i.e.":
			return false
		}
		// A character's initial belongs with the following surname.
		front := word[:len(word)-1]
		initial, n := utf8.DecodeRuneInString(front)
		return len(front) != n || !unicode.IsUpper(initial)
	}
	return false
}
