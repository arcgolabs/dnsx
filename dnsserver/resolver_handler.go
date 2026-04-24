package dnsserver

import (
	"context"

	"github.com/miekg/dns"
)

func (r *Resolver) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.Authoritative = false
	reply.RecursionAvailable = false

	if request.Opcode != dns.OpcodeQuery || len(request.Question) == 0 {
		reply.Rcode = dns.RcodeFormatError
		writeDNSMessage(r.logger, writer, reply)
		return
	}

	resolution, err := r.Resolve(context.Background(), request.Question[0])
	if err != nil {
		r.logger.Error("dns resolve failed", "err", err, "name", request.Question[0].Name, "type", request.Question[0].Qtype)
		reply.Rcode = dns.RcodeServerFailure
		writeDNSMessage(r.logger, writer, reply)
		return
	}

	reply.Rcode = resolution.RCode
	reply.Authoritative = resolution.Authoritative
	reply.Answer = resolution.Answer
	reply.Ns = resolution.Authority
	reply.Extra = resolution.Extra
	writeDNSMessage(r.logger, writer, reply)
}

var _ dns.Handler = (*Resolver)(nil)
