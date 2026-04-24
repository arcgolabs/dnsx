package dnsclient

import (
	"context"
	"log/slog"
	"time"

	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type Option func(*Client)

type Client struct {
	server  string
	network string
	logger  *slog.Logger
	client  *dns.Client
}

func New(server string, opts ...Option) *Client {
	client := &Client{
		server:  server,
		network: "udp",
		logger:  slog.Default(),
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

func WithLogger(logger *slog.Logger) Option {
	return func(client *Client) {
		if logger != nil {
			client.logger = logger
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
		return nil, 0, oops.In("dnsclient").
			With("op", "exchange_message").
			New("dns message is nil")
	}

	question := lo.Ternary(len(message.Question) > 0, message.Question[0].Name, "")
	qtype := lo.Ternary(len(message.Question) > 0, dns.TypeToString[message.Question[0].Qtype], "")
	c.logger.Debug(
		"dns client exchange started",
		"server", c.server,
		"network", c.network,
		"opcode", message.Opcode,
		"question", question,
		"type", qtype,
	)

	response, rtt, err := c.client.ExchangeContext(ctx, message, c.server)
	if err != nil {
		c.logger.Error(
			"dns client exchange failed",
			"server", c.server,
			"network", c.network,
			"opcode", message.Opcode,
			"question", question,
			"type", qtype,
			"err", err,
		)
		return nil, 0, oops.In("dnsclient").
			With("op", "exchange_message", "server", c.server, "network", c.network).
			Wrapf(err, "exchange dns message")
	}

	c.logger.Debug(
		"dns client exchange completed",
		"server", c.server,
		"network", c.network,
		"opcode", message.Opcode,
		"rcode", dns.RcodeToString[response.Rcode],
		"answer", len(response.Answer),
		"rtt", rtt,
	)

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
