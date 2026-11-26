package remote

import (
	"strings"
	"unicode"
)

func (d *document) place(book *Book, p span) string {
	on := d.onPage(p)
	where := book.Title + " / " + on[0].section
	if len(on) == 1 && on[0].study != nil {
		where += ", vv. " + on[0].study.verses
	}
	if book.Edition != "" {
		where += " (" + book.Edition + ")"
	}
	// An incipit distinguishes two places in the same chapter.
	words := strings.Fields(d.body[p.Start:p.End])
	if len(words) > 9 {
		return where + "\n" + strings.Join(words[:9], " ") + "…"
	}
	return where + "\n" + strings.Join(words, " ")
}

type gloss struct {
	verses  string // empty for general vocabulary
	word    string
	aliases string
	answer  string
}

// A gloss can only answer a question if its word occurs on the current page.
// Spellings without accents are accepted from a phone keypad.
var glossary = []gloss{
	{"1-3", "obscura", "oscura oscuridad", "«Obscura» es una grafía de «oscura». Dante escribe «selva oscura». Aranda conserva la imagen del bosque."},
	{"1-3", "viaje", "camino cammin diritta", "Aranda usa «viaje» para el curso de la vida y «camino recto» para «diritta via», la senda de la que Dante se apartó."},
	{"1-3", "selva", "forest wood bosque", "«Selva» es el bosque en el que Dante se encuentra perdido. Aranda conserva la palabra del italiano."},
	{"4-9", "espesa", "espeso densa", "«Espesa» describe una selva tan densa que cuesta atravesarla."},
	{"4-9", "pavor", "miedo", "«Pavor» es un miedo intenso. Aquí regresa cuando Dante recuerda la selva."},
	{"13-18", "cima", "spalle hombros", "Dante escribe «spalle», hombros. Donde el monte italiano tiene cuerpo, Aranda hace que Dante vea su «cima»."},
	{"19-21", "angustia", "pieta piedad", "Aranda traduce «pieta» por «angustia» para expresar el sufrimiento de la noche que acaba de pasar."},
	{"22-27", "piélago", "pelago mar", "«Piélago» es el mar abierto. El náufrago ya está en la orilla cuando se vuelve para mirarlo."},
	{"22-27", "playa", "orilla riva", "Aquí «playa» es la orilla a la que llega el náufrago. El italiano dice «riva»."},
	{"28-30", "solitaria playa", "playa piaggia ladera", "Dante escribe «piaggia diserta». Aquí es la ladera por la que sube. «Playa» arrastra la imagen del náufrago de la frase anterior."},
	{"", "ox-hides", "ox hide hides", "Ox-hides are the skins of cattle. They are one of the goods traded through Sulaco."},
	{"", "galleons", "galleon", "A galleon is a large sailing ship. Conrad pictures these heavy vessels waiting for enough wind to move."},
	{"", "daisy-chain", "daisy daisies", "A daisy-chain is a string of flowers linked by their stems. Alice is wondering whether to get up and pick them."},
}

func searchable(s string) string {
	s = strings.Map(func(r rune) rune {
		switch unicode.ToLower(r) {
		case 'á', 'à':
			return 'a'
		case 'é', 'è':
			return 'e'
		case 'í', 'ì':
			return 'i'
		case 'ó', 'ò':
			return 'o'
		case 'ú', 'ù', 'ü':
			return 'u'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)
	return " " + strings.Join(strings.Fields(s), " ") + " "
}

func (d *document) studyReply(p span, act action, query string) string {
	units := d.onPage(p)
	var answers []string
	if act == italian || act == workingVersion {
		for _, u := range units {
			if act == workingVersion {
				if u.study == nil || u.study.working == "" {
					return "The working Spanish covers Infierno I, vv. 13-30. 00 rereads this passage. 0 continues."
				}
				answers = append(answers, u.study.working)
			} else {
				if u.study == nil {
					return "No aligned Italian for this passage. 00 rereads it. 0 continues."
				}
				answers = append(answers, u.study.italian)
			}
		}
		reply := strings.Join(answers, "\n\n")
		if act == workingVersion {
			return "Versión de trabajo\n" + reply
		}
		return reply
	}

	page := d.body[p.Start:p.End]
	words, asked := searchable(page), searchable(query)
	var glosses []string
	for _, g := range glossary {
		if g.verses != "" {
			matched := false
			for _, u := range units {
				if u.study != nil && u.study.verses == g.verses {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if !strings.Contains(words, searchable(g.word)) {
			continue
		}
		if query == "" {
			glosses = append(glosses, g.answer)
			continue
		}
		if strings.Contains(asked, searchable(g.word)) {
			return g.answer
		}
		for _, alias := range strings.Fields(g.aliases) {
			if strings.Contains(asked, searchable(alias)) {
				return g.answer
			}
		}
	}
	for _, ref := range noteRef.FindAllStringSubmatchIndex(page, -1) {
		id := page[ref[2]:ref[3]]
		note := d.footnotes[id]
		if note == "" {
			continue
		}
		// The edition attaches the number directly to the word it glosses.
		before := strings.Fields(searchable(page[:ref[0]]))
		matchesWord := len(before) > 0 && strings.Contains(asked, searchable(before[len(before)-1]))
		if query == "" || strings.Contains(asked, searchable(id)) || matchesWord {
			answers = append(answers, "["+id+"] "+note)
		}
	}
	if len(answers) == 0 && query == "" {
		for _, u := range units {
			if u.study != nil {
				answers = append(answers, u.study.note)
			}
		}
	}
	if len(answers) == 0 {
		answers = glosses
	}
	if len(answers) == 0 {
		return "No reading note for that here. 00 rereads this passage. 0 continues."
	}
	return strings.Join(answers, "\n\n")
}
