// Package sms defines gateway-independent inbound messages, service routing,
// and outbound delivery. It does not send carrier SMS by itself.
package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Inbound is an already-authenticated/verified gateway message. A real adapter
// must verify the provider signature, normalize From, and supply a stable ID.
type Inbound struct {
	ID   string `json:"id,omitempty"`
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	Body string `json:"body"`
}

// Outbound is one logical response; a gateway decides encoding/segmentation.
type Outbound struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

type Handler interface {
	Handle(context.Context, Inbound) ([]Outbound, error)
}

// ErrUpstream signals a temporary upstream service failure; HTTP adapters return 502.
var ErrUpstream = errors.New("upstream temporarily unavailable")

type Sender interface {
	Send(context.Context, Outbound) error
}

// Dispatch selects a service from an explicit leading keyword, otherwise
// routing to Default. BOOK is the initial default; WEATHER, BUS, etc. may be
// registered without adding transport-specific code to the book reader.
type Dispatch struct {
	Default  Handler
	Services map[string]Handler
}

func (d Dispatch) Handle(ctx context.Context, in Inbound) ([]Outbound, error) {
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return nil, errors.New("empty message")
	}
	name, rest, hasRest := strings.Cut(body, " ")
	name = strings.ToLower(strings.TrimSuffix(name, ":"))
	if h := d.Services[name]; h != nil {
		if hasRest {
			in.Body = strings.TrimSpace(rest)
		} else {
			in.Body = ""
		}
		return h.Handle(ctx, in)
	}
	if d.Default == nil {
		return nil, fmt.Errorf("no service for %q", name)
	}
	return d.Default.Handle(ctx, in)
}

// Deliver is the explicit outbound gateway boundary. Without an installed
// Sender nothing here implies that an SMS has actually been sent.
func Deliver(ctx context.Context, sender Sender, msgs []Outbound) error {
	if sender == nil {
		return errors.New("no outbound gateway configured")
	}
	for _, msg := range msgs {
		if err := sender.Send(ctx, msg); err != nil {
			return fmt.Errorf("sending to %q: %w", msg.To, err)
		}
	}
	return nil
}
