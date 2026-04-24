package dnsserver

import (
	"context"
	"fmt"

	"github.com/miekg/dns"
	"github.com/samber/lo"
)

func (s *Server) serveDNS(writer dns.ResponseWriter, request *dns.Msg) {
	switch request.Opcode {
	case dns.OpcodeQuery:
		if s.resolver == nil {
			reply := new(dns.Msg)
			reply.SetRcode(request, dns.RcodeRefused)
			_ = writer.WriteMsg(reply)
			return
		}
		s.resolver.ServeDNS(writer, request)
	case dns.OpcodeUpdate:
		s.serveUpdate(writer, request)
	default:
		reply := new(dns.Msg)
		reply.SetRcode(request, dns.RcodeNotImplemented)
		_ = writer.WriteMsg(reply)
	}
}

func (s *Server) serveUpdate(writer dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.Authoritative = true
	reply.RecursionAvailable = false

	if s.repo == nil {
		reply.Rcode = dns.RcodeRefused
		_ = writer.WriteMsg(reply)
		return
	}

	if len(request.Question) != 1 || request.Question[0].Qclass != dns.ClassINET || request.Question[0].Qtype != dns.TypeSOA {
		reply.Rcode = dns.RcodeFormatError
		_ = writer.WriteMsg(reply)
		return
	}

	zone, err := NormalizeZoneName(request.Question[0].Name)
	if err != nil {
		reply.Rcode = dns.RcodeFormatError
		_ = writer.WriteMsg(reply)
		return
	}

	if len(request.Answer) > 0 {
		reply.Rcode = dns.RcodeNotImplemented
		_ = writer.WriteMsg(reply)
		return
	}

	if err := s.applyUpdate(context.Background(), zone, request.Ns); err != nil {
		s.logger.Error("dns update failed", "zone", zone, "err", err)
		reply.Rcode = dns.RcodeServerFailure
		_ = writer.WriteMsg(reply)
		return
	}

	reply.Rcode = dns.RcodeSuccess
	_ = writer.WriteMsg(reply)
}

func (s *Server) applyUpdate(ctx context.Context, zone string, updates []dns.RR) error {
	for _, update := range updates {
		if err := s.applyUpdateRR(ctx, zone, update); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) applyUpdateRR(ctx context.Context, zone string, update dns.RR) error {
	header := update.Header()
	switch {
	case header.Class == dns.ClassANY && header.Rrtype == dns.TypeANY:
		return s.deleteName(ctx, zone, header.Name)
	case header.Class == dns.ClassANY:
		return s.deleteRRSet(ctx, zone, header.Name, header.Rrtype)
	case header.Class == dns.ClassNONE:
		record, err := RecordFromRR(zone, update)
		if err != nil {
			return err
		}
		record.Class = dns.ClassINET
		return s.repo.DeleteRecord(ctx, record)
	default:
		record, err := RecordFromRR(zone, update)
		if err != nil {
			return err
		}
		return s.repo.SaveRecord(ctx, record)
	}
}

func (s *Server) deleteName(ctx context.Context, zone string, name string) error {
	records, err := s.repo.LookupAll(ctx, zone, name, dns.ClassANY)
	if err != nil {
		return fmt.Errorf("lookup records for delete name %q: %w", name, err)
	}

	return lo.Reduce(records, func(acc error, record Record, _ int) error {
		if acc != nil {
			return acc
		}
		return s.repo.DeleteRecord(ctx, record)
	}, nil)
}

func (s *Server) deleteRRSet(ctx context.Context, zone string, name string, rrtype uint16) error {
	records, err := s.repo.Lookup(ctx, zone, name, rrtype, dns.ClassANY)
	if err != nil {
		return fmt.Errorf("lookup rrset for delete %q type=%d: %w", name, rrtype, err)
	}

	return lo.Reduce(records, func(acc error, record Record, _ int) error {
		if acc != nil {
			return acc
		}
		return s.repo.DeleteRecord(ctx, record)
	}, nil)
}
