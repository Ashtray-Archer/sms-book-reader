// Command remote-reader hosts the remote-content reader without assuming any
// carrier SMS gateway. Its /inbound endpoint returns would-send responses.
package main

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/isomorphisms/sms-book-reader/internal/remote"
	"github.com/isomorphisms/sms-book-reader/internal/sms"
)

//go:embed books.json
var files embed.FS

func usage() {
	fmt.Fprintln(os.Stderr, "usage: remote-reader [chat [SENDER]] | books | message SENDER 'hi' | serve [127.0.0.1:8788]")
}

func reply(handler sms.Handler, sender, body string, out io.Writer) error {
	messages, err := handler.Handle(context.Background(), sms.Inbound{From: sender, Body: body})
	if err != nil {
		return err
	}
	for _, m := range messages {
		if _, err := fmt.Fprintln(out, m.Body); err != nil {
			return err
		}
	}
	return nil
}

// A local conversation exercises the very same turns as /inbound. On a
// terminal, Return is a convenient alias for the flip phone's zero key.
func chat(handler sms.Handler, sender string, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	body := "hi"
	for {
		if err := reply(handler, sender, body, out); err != nil {
			fmt.Fprintln(out, err)
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		body = strings.TrimSpace(scanner.Text())
		if strings.EqualFold(body, "quit") {
			return nil
		}
		if body == "" {
			body = "0"
		}
	}
}

func main() {
	data, err := files.ReadFile("books.json")
	if err != nil {
		log.Fatal(err)
	}
	catalog, err := remote.ParseCatalog(data)
	if err != nil {
		log.Fatal(err)
	}
	root := os.Getenv("SMS_READER_HOME")
	if root == "" {
		userDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		root = filepath.Join(userDir, ".local", "state", "sms-book-reader")
	}
	root = filepath.Join(root, "remote") // independent of existing mock bookmarks
	client := &http.Client{Timeout: 15 * time.Second}
	reader := &remote.Service{Catalog: catalog, Texts: &remote.Texts{CacheDir: root, Client: client}, Store: remote.Store{Dir: root}}
	dispatch := sms.Dispatch{Default: reader, Services: map[string]sms.Handler{"book": reader}}
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"chat"}
	}
	switch args[0] {
	case "chat":
		sender := "local"
		if len(args) == 2 {
			sender = args[1]
		}
		if len(args) > 2 {
			usage()
			os.Exit(2)
		}
		if err := chat(dispatch, sender, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "books":
		fmt.Println(catalog.List())
	case "message":
		if len(args) < 3 {
			usage()
			os.Exit(2)
		}
		if err := reply(dispatch, args[1], strings.Join(args[2:], " "), os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "serve":
		addr := "127.0.0.1:8788"
		if len(args) > 1 {
			addr = args[1]
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			log.Fatal(err)
		}
		token := os.Getenv("SMS_BOOK_HTTP_TOKEN")
		if token == "" && host != "127.0.0.1" && host != "localhost" && host != "::1" {
			log.Fatal("nonloopback HTTP listener requires SMS_BOOK_HTTP_TOKEN; this is not a verified SMS gateway")
		}
		log.Printf("remote reader HTTP integration ingress on %s (no carrier delivery)", addr)
		log.Fatal(http.ListenAndServe(addr, sms.HTTPHandler(dispatch, token)))
	default:
		usage()
		os.Exit(2)
	}
}
