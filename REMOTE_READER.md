# Reading by text

Send `hi` for the shelf. Reply with the number of a book and its first passage comes back immediately. If you already have a bookmark in that book, the reply is the passage you last saw. On a later visit, `0` continues your active book from the first unread word.

| Text | Reply |
| --- | --- |
| `0`, `next`, `siguiente` | The next passage |
| `00`, `again` | The passage you just read |
| `B` | The preceding passage |
| `#` | The numbered shelf. Each book keeps its own place. |
| `*` or a question such as `piélago?` | A reading note, a word on this page, or a footnote from the edition, when available |
| `IT` | The Italian for this passage of *Infierno*, with its verse breaks |
| `OTRA` | The working Spanish translation for this passage, when the Dante project has one |
| `WHERE` | The chapter or canto and the opening words of your passage |
| `LONGER`, `SHORTER` | A longer or shorter turn, starting with the next unread words |
| `?` | The controls for the active book |

A question, a comparison, and a visit to the shelf all leave your place intact. Silence pauses the reading indefinitely. Selecting a title by name and the earlier conversational commands also work.

## A turn in *Infierno*

`3`

> A la mitad del viaje de nuestra vida me encontré en una selva obscura, por haberme apartado del camino recto.

`IT`

> Nel mezzo del cammin di nostra vita\
> mi ritrovai per una selva oscura,\
> ché la diritta via era smarrita.

`0`

> ¡Ah! Cuán penoso me sería decir lo salvaje, áspera y espesa que era esta selva, cuyo recuerdo renueva mi pavor, pavor tan amargo, que la muerte no lo es tanto. Pero antes de hablar del bien que allí encontré, revelaré las demás cosas que he visto.

The default Spanish text is Manuel Aranda y Sanjuán’s whole *Infierno*. Its numbered edition notes are available where their markers occur. The Italian comparisons and reading notes cover canto I, verses 1–30. The project’s working Spanish covers verses 13–30. The Spanish prose and Italian tercets are paired by meaning. For example, on verses 28–30 the question `playa?` brings up *piaggia diserta* and the reason the reader’s working version says *ladera*. Questions use the material associated with the displayed passage. The service says when an answer is unavailable. [Edition, Italian, working translations and reading notes](https://github.com/walnut-burgundy/mi-Dante/tree/40a1a17e9ad358d44300c0f4ebdb8e43444017e4)

Prose turns at sentence ends where possible. A very long sentence can break at a clause or a word. Verse retains its line breaks. The ordinary passage reply contains the literature alone. Length is measured per reading turn. A carrier may use multiple SMS segments for a turn.

## Try a conversation

From a checkout with Go 1.23 or later, run `go run ./cmd/remote-reader`. This opens the shelf or your most recent passage. Type replies as you would on the phone. Return alone means next in the terminal. `quit` or end of input closes it.

For a separate test reader, use `go run ./cmd/remote-reader chat example`. The single-message interface is `go run ./cmd/remote-reader message example '0'`. Both use the same handler and bookmarks as HTTP. The default state directory is `~/.local/state/sms-book-reader/remote`. Set `SMS_READER_HOME` to choose its parent.

The first selection downloads the pinned edition over HTTPS. The reader verifies its Git blob before writing a private local cache. Later visits can use that cache offline. The catalog also includes the Gutenberg editions of [Nostromo](https://www.gutenberg.org/ebooks/2021) and [Alice’s Adventures in Wonderland](https://www.gutenberg.org/ebooks/11).

## SMS transport

`go run ./cmd/remote-reader serve 127.0.0.1:8788` starts the HTTP integration endpoint. A gateway submits `POST /inbound` with a JSON body such as `{"id":"message-001","from":"example","body":"hi"}`. The reply contains an `outbound` array of `{to,body}`. Set `SMS_BOOK_HTTP_TOKEN` for bearer authentication. A non-loopback listener requires a token. `GET /health` reports availability.

A live phone number still needs a carrier adapter for authenticated inbound messages, outbound delivery, SMS segmentation and delivery receipts. The integration endpoint returns the proposed response to its caller. The reader runs one process per state directory. It saves each sender’s state in a private file by atomic replacement and remembers the last 32 inbound IDs for retries. A retry of an identical message ID and body returns the original response. A carrier adapter needs a durable outbox to recover a failed outbound send after a bookmark has been stored.

Bookmarks identify the displayed byte range in the normalized, pinned edition. On first opening a book from the previous remote reader, its 240-character page index is converted back to the words it displayed. The next turn continues at the first unread word, even if the old boundary fell inside a sentence. The earlier fixture-based jog keeps its own state.

## Verification

`go test -race ./...` exercises keypad conversations, reading boundaries, both Spanish versions, Italian, footnotes, interruption and resumption, migration, switching books, retries, source verification and the HTTP endpoint. `go vet ./...` checks the packages. To download and verify all three complete pinned sources, run `SMS_READER_LIVE=1 go test -run TestLivePinnedBooks -v ./internal/remote`.
