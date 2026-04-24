package dnsserver

import (
	"log/slog"

	"github.com/miekg/dns"
)

func writeDNSMessage(logger *slog.Logger, writer dns.ResponseWriter, message *dns.Msg) {
	if err := writer.WriteMsg(message); err != nil && logger != nil {
		logger.Error("write dns message failed", "err", err)
	}
}
