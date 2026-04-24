package dnsclient

import (
	"context"
	"fmt"
	"time"

	"github.com/miekg/dns"
	"github.com/samber/lo"
)

type Option func(*Client)

type Client struct {
	server  string
	network string
	client  *dns.Client
}

func New(server string, opts ...Option) *Client {
	client := &Client{
		server:  server,
		network: "udp",
		client: &dns.Client{
			Net:     "udp",
			Timeout: 5 * time.Second,
		},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}

	client.client.Net = client.network
	return client
}

func NewClient(server string, opts ...Option) *Client {
	return New(server, opts...)
}

func WithNetwork(network string) Option {
	return func(client *Client) {
		if network != "" {
			client.network = network
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(client *Client) {
		if timeout > 0 {
			client.client.Timeout = timeout
		}
	}
}

func (c *Client) Exchange(ctx context.Context, name string, qtype uint16) (*dns.Msg, time.Duration, error) {
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(name), qtype)

	return c.ExchangeMessage(ctx, message)
}

func (c *Client) ExchangeMessage(ctx context.Context, message *dns.Msg) (*dns.Msg, time.Duration, error) {
	if message == nil {
		return nil, 0, fmt.Errorf("dns message is nil")
	}

	response, rtt, err := c.client.ExchangeContext(ctx, message, c.server)
	if err != nil {
		return nil, 0, fmt.Errorf("dns exchange with %s over %s: %w", c.server, c.network, err)
	}

	return response, rtt, nil
}

func (c *Client) Lookup(ctx context.Context, name string, qtype uint16) ([]dns.RR, error) {
	response, _, err := c.Exchange(ctx, name, qtype)
	if err != nil {
		return nil, err
	}

	return response.Answer, nil
}

func (c *Client) LookupA(ctx context.Context, name string) ([]string, error) {
	answer, err := c.Lookup(ctx, name, dns.TypeA)
	if err != nil {
		return nil, err
	}

	return lo.FilterMap(answer, func(rr dns.RR, _ int) (string, bool) {
		record, ok := rr.(*dns.A)
		if !ok {
			return "", false
		}

		return record.A.String(), true
	}), nil
}

func (c *Client) UpdateAdd(ctx context.Context, zone string, records ...dns.RR) (*dns.Msg, time.Duration, error) {
	message := new(dns.Msg)
	message.SetUpdate(dns.Fqdn(zone))
	message.Insert(records)
	return c.ExchangeMessage(ctx, message)
}

func (c *Client) UpdateRemove(ctx context.Context, zone string, records ...dns.RR) (*dns.Msg, time.Duration, error) {
	message := new(dns.Msg)
	message.SetUpdate(dns.Fqdn(zone))
	message.Remove(records)
	return c.ExchangeMessage(ctx, message)
}

func (c *Client) UpdateRemoveRRSet(ctx context.Context, zone string, name string, rrtype uint16) (*dns.Msg, time.Duration, error) {
	message := new(dns.Msg)
	message.SetUpdate(dns.Fqdn(zone))
	message.RemoveRRset([]dns.RR{
		&dns.ANY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: rrtype,
				Class:  dns.ClassANY,
				Ttl:    0,
			},
		},
	})
	return c.ExchangeMessage(ctx, message)
}

func (c *Client) UpdateRemoveName(ctx context.Context, zone string, name string) (*dns.Msg, time.Duration, error) {
	message := new(dns.Msg)
	message.SetUpdate(dns.Fqdn(zone))
	message.RemoveName([]dns.RR{
		&dns.ANY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeANY,
				Class:  dns.ClassANY,
				Ttl:    0,
			},
		},
	})
	return c.ExchangeMessage(ctx, message)
}
