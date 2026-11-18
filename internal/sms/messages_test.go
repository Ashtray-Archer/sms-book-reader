package sms

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type handlerFunc func(context.Context, Inbound) ([]Outbound, error)

func (h handlerFunc) Handle(ctx context.Context, in Inbound) ([]Outbound, error) { return h(ctx, in) }

type senderFunc func(context.Context, Outbound) error

func (s senderFunc) Send(ctx context.Context, out Outbound) error { return s(ctx, out) }

func TestDispatchAndSenderBoundary(t *testing.T) {
	book := handlerFunc(func(_ context.Context, in Inbound) ([]Outbound, error) {
		return []Outbound{{To: in.From, Body: "book:" + in.Body}}, nil
	})
	weather := handlerFunc(func(_ context.Context, in Inbound) ([]Outbound, error) {
		return []Outbound{{To: in.From, Body: "weather:" + in.Body}}, nil
	})
	d := Dispatch{Default: book, Services: map[string]Handler{"weather": weather, "book": book}}
	for input, want := range map[string]string{"continue": "book:continue", "book read alice": "book:read alice", "weather Detroit": "weather:Detroit"} {
		out, err := d.Handle(context.Background(), Inbound{From: "ada", Body: input})
		if err != nil || len(out) != 1 || out[0].Body != want {
			t.Fatalf("%q: %+v %v", input, out, err)
		}
	}
	out, _ := d.Handle(context.Background(), Inbound{From: "ada", Body: "continue"})
	if err := Deliver(context.Background(), nil, out); err == nil {
		t.Fatal("unconfigured gateway should refuse delivery")
	}
	var sent []string
	if err := Deliver(context.Background(), senderFunc(func(_ context.Context, m Outbound) error { sent = append(sent, m.Body); return nil }), out); err != nil || len(sent) != 1 || sent[0] != "book:continue" {
		t.Fatalf("delivery boundary: %v %v", sent, err)
	}
	if err := Deliver(context.Background(), senderFunc(func(_ context.Context, _ Outbound) error { return errors.New("down") }), out); err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("outbound errors swallowed: %v", err)
	}
}
