# Remote SMS Book Reader: Go acceptance candidate

**No carrier SMS messages have been sent.** This is a remote-source, server-side
book reader that accepts transport-independent inbound messages and returns
outbound *intent*. Its first gateway is an authenticated, local HTTP integration
endpoint, not a verified carrier webhook. Existing `main.go` and mock reader
on `feature/go-mock-reader` are intentionally unchanged.

## Provenance and scope

The *two available works* are **Nostromo** (Joseph Conrad, Project Gutenberg
#2021) and **Alice's Adventures in Wonderland** (Lewis Carroll, Project
Gutenberg #11). Both are listed by Project Gutenberg as public domain in the US.
A remote GITenberg GitHub archive contains the source plaintext. The catalog
pins a specific source repository **commit** and **Git blob SHA-1** for each.
A checksum mismatch, partial download, missing Gutenberg end marker, or
unavailable remote host fails without advancing a bookmark. First successful
fetch saves the *verified* text under `SMS_READER_HOME/remote/books/` for future
restarts. No copyrighted mock text is presented as an actual book.

- Conrad: https://www.gutenberg.org/ebooks/2021
- Carroll: https://www.gutenberg.org/ebooks/11
- GITenberg archive: https://github.com/GITenberg

Public-domain status is jurisdiction dependent. The separate GITenberg source
and Gutenberg license files remain available at the attributed repository.
This project does not copy the copyrighted placeholder translations or the
mock reading list into the remote catalog.

## Try the actual service

Run from the repository branch `feature/go-mock-reader` **after applying this
proposed patch**; requires Go 1.23 or later:

```sh
export SMS_READER_HOME="$(mktemp -d)"
go test ./...
go run ./cmd/remote-reader books
go run ./cmd/remote-reader message reader1 'read nostromo'
go run ./cmd/remote-reader message reader1 'more'
go run ./cmd/remote-reader message reader1 'resume'
go run ./cmd/remote-reader message reader1 'read alice'
go run ./cmd/remote-reader message reader1 'read nostromo'
go run ./cmd/remote-reader message reader1 'start over'
```

`read nostromo` fetches actual Conrad text **over HTTPS** from a pinned GitHub
source the first time the book is requested (unless an intact cache exists).
Subsequent calls use the source cache. `MORE`, `NEXT`, `CONTINUE`, `GO ON`,
`KEEP READING`, `RESUME`, `AGAIN`, `READ <title>`, `START OVER`, `BOOKS`,
and common conversational variants are understood. First-time `START READING`
uses Nostromo. Each reader has **one durable position per book**. `RESUME`
repeats the saved passage; `MORE` advances. The output is at most 240 Unicode
code points of book text plus a brief title/progress prefix per logical reply.
It does **not** yet enforce GSM-7/UCS-2 carrier segment length; the actual
provider adapter must handle encoding, segmentation and delivery receipts.

## Provider-neutral HTTP boundary

```sh
export SMS_BOOK_HTTP_TOKEN='use-a-secret-value'
go run ./cmd/remote-reader serve 127.0.0.1:8788
```

In another terminal:

```sh
curl -sS http://127.0.0.1:8788/inbound \
  -H 'Authorization: Bearer use-a-secret-value' \
  -H 'Content-Type: application/json' \
  --data '{"id":"message-001","from":"test-reader","body":"read nostromo"}'
```

The response is JSON with an `outbound` array of `{to,body}`. It **does not**
send the response to a phone. `GET /health` is the only other endpoint.
Without a token, the CLI refuses a non-loopback listener; nonloopback with a
token still requires TLS termination, access controls, and a provider signature
validator before exposure. Do not put real phone numbers into an untrusted
public webhook.

The interfaces in `internal/sms` are meant for reuse by later transit, weather,
reminder, or remote-model services:

```go
type Inbound struct { ID, From, To, Body string }
type Outbound struct { To, Body string }
type Handler interface { Handle(context.Context, Inbound) ([]Outbound, error) }
type Sender interface { Send(context.Context, Outbound) error }
```

`Dispatch` routes `BOOK ...` to the book service, has a default service for
conversational requests, and accepts later named services without depending
on the carrier. `Deliver` refuses to claim delivery when no `Sender` exists.
A real SMS adapter must (1) authenticate the provider request, (2) normalize
sender identity, (3) supply a stable inbound message ID, (4) map replies to a
carrier sender/recipient, (5) segment SMS safely, and (6) implement a durable
outbox plus provider receipt/reconciliation to avoid losing responses if an
outbound send fails after the book position is saved. **These are unimplemented.**

## Durability and verification

State is JSON in a hashed-sender filename, mode `0600`, atomically replaced
from a same-directory temporary file with fsync and rename. One bookmark per
book and sender; duplicate inbound IDs replay the original reply (most recent
32 IDs) instead of advancing twice. This works for one running service
process per state directory, **not** simultaneously running processes. Existing
mock bookmark indices are incompatible with the real book's chunk numbering;
this service keeps them in a separate `remote/` directory and does not pretend
to migrate them.

`go test -race ./...` uses actual public-domain opening paragraphs in test
fixtures, served by `httptest` as a simulated HTTP book host. It proves HTTP
fetch, boundary stripping, real passage text, bounded segmentation, catalog
selection, reader isolation, restart/bookmark recovery, cached-text reuse while
the source is offline, checksum mismatch refusal, duplicate-ID replay, and the
authenticated HTTP integration boundary. It **does not** prove real GitHub
network access from a deployed host or carrier SMS delivery. Source attribution
and checksum identities were checked against the public GITenberg repository.

Proposed addition: `.github/workflows/remote-reader.yml` runs the race tests,
`go vet`, and a host binary build on an approved future push. It has no release
or deployment step.

Optional live source qualification, on a host with network access:

```sh
SMS_READER_LIVE=1 go test -run TestLivePinnedBooks -v ./internal/remote
```

This separate test downloads each entire pinned source, verifies its Git blob
identity, strips the Gutenberg wrapper, and confirms that the opening passage
contains Conrad's or Carroll's actual words. It is skipped in normal tests
and has **not** passed in the restricted local execution environment.
