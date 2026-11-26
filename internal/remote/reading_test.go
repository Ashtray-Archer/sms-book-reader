package remote

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/isomorphisms/sms-book-reader/internal/sms"
)

func TestKeypadConversation(t *testing.T) {
	r, server, requests := testService(t)
	defer server.Close()
	say := func(body string) string { return send(t, r, "reader", "", body) }
	shelf := say("hi")
	if !strings.Contains(shelf, "3 Infierno") || !strings.Contains(shelf, "0 next") || say("0") != shelf || requests.Load() != 0 {
		t.Fatalf("fresh sender got %q with %d fetches", shelf, requests.Load())
	}
	first := say("3")
	if first != "A la mitad del viaje de nuestra vida me encontré en una selva obscura, por haberme apartado del camino recto." {
		t.Fatal(first)
	}
	original := "Nel mezzo del cammin di nostra vita\nmi ritrovai per una selva oscura,\nché la diritta via era smarrita."
	if got := say("it"); got != original {
		t.Fatal(got)
	}
	if got := say("¿qué significa obscura?"); !strings.Contains(got, "grafía") {
		t.Fatal(got)
	}
	if got := say("una pregunta sin respuesta?"); !strings.Contains(got, "No reading note") {
		t.Fatal(got)
	}
	if got := say("*"); !strings.Contains(got, "«camino»") {
		t.Fatal(got)
	}
	if got := say("where"); !strings.Contains(got, "Canto 1, vv. 1-3 (Aranda y Sanjuán)") {
		t.Fatal(got)
	}
	say("pause")
	say("?")
	if say("#") != shelf || say("00") != first {
		t.Fatal("detour moved the bookmark")
	}
	second := say("siguiente")
	if !strings.HasPrefix(second, "¡Ah! Cuán penoso") || !strings.HasSuffix(second, "cosas que he visto.") {
		t.Fatal(second)
	}
	if italian := say("original"); len(strings.Split(italian, "\n\n")) != 2 || !strings.HasSuffix(italian, "ch’i’ v’ho scorte.") {
		t.Fatal(italian)
	}
	say("2")
	if got := say("3"); got != second {
		t.Fatal("switching titles lost the Spanish passage")
	}
	server.Close()
	r = &Service{Catalog: r.Catalog, Texts: &Texts{CacheDir: r.Store.Dir}, Store: r.Store}
	if say("hello") != second {
		t.Fatal("restart lost the Spanish passage")
	}
	third := say("0")
	if !strings.HasPrefix(third, "No sé decir fijamente") {
		t.Fatal(third)
	}
	if say("b") != second || say("0") != third {
		t.Fatal("back/next lost the text")
	}
	longPage := say("un poco más largo")
	if !strings.HasPrefix(longPage, "Pero al llegar") || !strings.Contains(longPage, "lago de mi corazón") {
		t.Fatal(longPage)
	}
	nextPage := say("shorter")
	if !strings.HasPrefix(nextPage, "y del mismo modo") {
		t.Fatal(nextPage)
	}
	if say("b") != longPage || say("shorter") != nextPage {
		t.Fatal("a change of length lost the previous range")
	}
	if got := say("pielago?"); !strings.Contains(got, "mar abierto") {
		t.Fatal(got)
	}
	if got := say("playa?"); !strings.Contains(got, "orilla") {
		t.Fatal(got)
	}
	say("0")
	if got := say("otra"); !strings.HasPrefix(got, "Versión de trabajo\n") || !strings.Contains(got, "ladera desierta") {
		t.Fatal(got)
	}
	if got := say("playa?"); !strings.Contains(got, "ladera") {
		t.Fatal(got)
	}
	say("0")
	if got := say("it"); !strings.Contains(got, "No aligned Italian") {
		t.Fatal(got)
	}
}

func TestEditionFootnotesStayOutsideReading(t *testing.T) {
	r, s, _ := testService(t)
	defer s.Close()
	say := func(body string) string { return send(t, r, "reader", "", body) }
	page := say("3")
	var sawNote, sawSecondCanto bool
	for turns := 0; turns < 300; turns++ {
		if strings.HasPrefix(page, "End of ") {
			break
		}
		if strings.Contains(page, "Ilustración") || strings.Contains(page, "Can Grande della Scala") {
			t.Fatal(page)
		}
		if strings.Contains(page, "Lebrel[1]") {
			footnote := say("nota 1")
			if !strings.Contains(footnote, "Can Grande della Scala") || say("Lebrel?") != footnote || say("00") != page {
				t.Fatal(footnote)
			}
			sawNote = true
		}
		if strings.Contains(say("where"), "Canto 2") {
			sawSecondCanto = true
		}
		page = say("0")
	}
	if !sawNote || !sawSecondCanto || !strings.HasPrefix(page, "End of Infierno") {
		t.Fatalf("note %v, second canto %v, last reply %q", sawNote, sawSecondCanto, page)
	}
	last := say("00")
	if say("0") != page || say("00") != last {
		t.Fatal("end changed the place")
	}
}

func TestEarlierReleaseBookmarkInsideSentence(t *testing.T) {
	r, s, _ := testService(t)
	defer s.Close()
	// Old indices count the 240-rune word-wrapped pages. The first page
	// stopped inside Conrad's opening sentence, before "trade in ox-hides".
	const paragraph = "In the time of Spanish rule, and for many years afterwards, the town of Sulaco--the luxuriant beauty of the orange gardens bears witness to its antiquity--had never been commercially anything more important than a coasting port with a fairly large local trade in ox-hides and indigo."
	legacy := splitWords(paragraph, 240)
	if err := r.Store.Save("reader", &State{Current: "nostromo", Positions: map[string]int{"nostromo": 0, "alice": 1}}); err != nil {
		t.Fatal(err)
	}
	say := func(body string) string { return send(t, r, "reader", "", body) }
	if got := say("00"); got != legacy[0] {
		t.Fatal(got)
	}
	if got := say("0"); legacy[0]+" "+got != paragraph {
		t.Fatalf("migration skipped or repeated words: %s", got)
	}
	state, err := r.Store.Load("reader")
	if err != nil || len(state.Positions) != 1 || state.Positions["alice"] != 1 || state.Bookmarks["nostromo"].Source == "" {
		t.Fatalf("%+v, %v", state, err)
	}
	if got := say("0"); !strings.HasPrefix(got, "The clumsy") {
		t.Fatal(got)
	}
	if got := say("2"); !strings.Contains(got, "thought Alice") {
		t.Fatal(got)
	}
	restarted := &Service{Catalog: r.Catalog, Texts: &Texts{CacheDir: r.Store.Dir}, Store: r.Store}
	if got := send(t, restarted, "reader", "", "1"); !strings.HasPrefix(got, "The clumsy") {
		t.Fatal(got)
	}
}

func TestTurnBoundariesAndVerse(t *testing.T) {
	verse := "How doth the little crocodile\nImprove his shining tail,\nAnd pour the waters of the Nile\nOn every golden scale!"
	prose := "Mr. White stopped. “Did you hear?” He waited."
	token := strings.Repeat("界", 450)
	raw := "*** START OF THIS PROJECT GUTENBERG EBOOK TEST ***\nCHAPTER I\n" + prose + "\n\n    " + strings.ReplaceAll(verse, "\n", "\n    ") + "\n\n" + token + "\n\nCHAPTER II\nAfterwards.\n*** END OF THIS PROJECT GUTENBERG EBOOK TEST ***"
	doc, err := parseDocument([]byte(raw), &Book{StartAfter: "CHAPTER I"})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.next(0, short)
	if doc.body[p.Start:p.End] != "Mr. White stopped." {
		t.Fatal(p)
	}
	p, _ = doc.next(p.End, short)
	if doc.body[p.Start:p.End] != "“Did you hear?”" {
		t.Fatal(p)
	}
	p, _ = doc.next(0, usual)
	if doc.body[p.Start:p.End] != prose {
		t.Fatal(p)
	}
	p, _ = doc.next(p.End, usual)
	if doc.body[p.Start:p.End] != verse {
		t.Fatal("stanza line breaks were folded")
	}
	var parts []string
	end := 0
	for i := 0; ; i++ {
		page, ok := doc.next(end, []pace{long, short, usual}[i%3])
		if !ok {
			break
		}
		if !doc.valid(page) || strings.TrimSpace(doc.body[end:page.Start]) != "" || page.End <= end {
			t.Fatal(page)
		}
		parts = append(parts, strings.Join(strings.Fields(doc.body[page.Start:page.End]), ""))
		end = page.End
	}
	if strings.Join(parts, "") != strings.Join(strings.Fields(prose+verse+token+"Afterwards."), "") {
		t.Fatal("pace or chapter transition lost text")
	}
}

func TestShippedShelfFitsOnePlainSMS(t *testing.T) {
	data, err := os.ReadFile("../../cmd/remote-reader/books.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	shelf := c.List()
	if len(shelf) > 160 {
		t.Fatalf("shelf length %d", len(shelf))
	}
	// The shipped shelf uses only one-septet ASCII punctuation.
	for _, r := range shelf {
		if r > 126 || strings.ContainsRune("^{}\\[~]|"+string(rune(96)), r) {
			t.Fatal(shelf)
		}
	}
}

func TestBadBookmarkFailsWithoutMoving(t *testing.T) {
	r, s, _ := testService(t)
	defer s.Close()
	send(t, r, "reader", "", "3")
	st, err := r.Store.Load("reader")
	if err != nil {
		t.Fatal(err)
	}
	mark := st.Bookmarks["infierno"]
	mark.Page = span{1, 1}
	st.Bookmarks["infierno"] = mark
	if err := r.Store.Save("reader", st); err != nil {
		t.Fatal(err)
	}
	_, err = r.Handle(context.Background(), sms.Inbound{From: "reader", Body: "where"})
	if err == nil {
		t.Fatal("invalid position accepted")
	}
	recovered := send(t, r, "reader", "", "start over")
	if !strings.HasPrefix(recovered, "A la mitad") {
		t.Fatal(recovered)
	}
}
