package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestServerStartStop(t *testing.T) {
	t.Parallel()

	server := NewServer(
		Config{Listen: "127.0.0.1:0"},
		dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
			reply := new(dns.Msg)
			reply.SetReply(request)
			_ = writer.WriteMsg(reply)
		}),
	)

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	if server.UDPAddr() == "" {
		t.Fatal("expected udp address after start")
	}
	if server.TCPAddr() == "" {
		t.Fatal("expected tcp address after start")
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("stop server: %v", err)
	}
}
