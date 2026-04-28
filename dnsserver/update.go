package dnsserver

import (
	"context"

	"github.com/miekg/dns"
	"github.com/samber/oops"
)

func (s *Server) serveDNS(writer dns.ResponseWriter, request *dns.Msg) {
	switch request.Opcode {
	case dns.OpcodeQuery:
		if s.resolver == nil {
			reply := new(dns.Msg)
			reply.SetRcode(request, dns.RcodeRefused)
			writeDNSMessage(s.logger, writer, reply)
			return
		}
		s.resolver.ServeDNS(writer, request)
	case dns.OpcodeUpdate:
		s.serveUpdate(writer, request)
	default:
		reply := new(dns.Msg)
		reply.SetRcode(request, dns.RcodeNotImplemented)
		writeDNSMessage(s.logger, writer, reply)
	}
}

func (s *Server) serveUpdate(writer dns.ResponseWriter, request *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetReply(request)
	reply.Authoritative = true
	reply.RecursionAvailable = false

	if s.repo == nil {
		reply.Rcode = dns.RcodeRefused
		writeDNSMessage(s.logger, writer, reply)
		return
	}

	if len(request.Question) != 1 || request.Question[0].Qclass != dns.ClassINET || request.Question[0].Qtype != dns.TypeSOA {
		reply.Rcode = dns.RcodeFormatError
		writeDNSMessage(s.logger, writer, reply)
		return
	}

	zone, err := NormalizeZoneName(request.Question[0].Name)
	if err != nil {
		reply.Rcode = dns.RcodeFormatError
		writeDNSMessage(s.logger, writer, reply)
		return
	}

	if len(request.Answer) > 0 {
		reply.Rcode = dns.RcodeNotImplemented
		writeDNSMessage(s.logger, writer, reply)
		return
	}

	if err := s.applyUpdate(context.Background(), zone, request.Ns); err != nil {
		s.logger.Error("dns update failed", "zone", zone, "err", err)
		reply.Rcode = dns.RcodeServerFailure
		writeDNSMessage(s.logger, writer, reply)
		return
	}

	reply.Rcode = dns.RcodeSuccess
	writeDNSMessage(s.logger, writer, reply)
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
	switch header.Class {
	case dns.ClassANY:
		return s.applyAnyClassUpdate(ctx, zone, header)
	case dns.ClassNONE:
		return s.applyDeleteRecordUpdate(ctx, zone, update)
	default:
		return s.applySaveRecordUpdate(ctx, zone, update)
	}
}

func (s *Server) applyAnyClassUpdate(ctx context.Context, zone string, header *dns.RR_Header) error {
	if header.Rrtype == dns.TypeANY {
		return s.deleteName(ctx, zone, header.Name)
	}

	return s.deleteRRSet(ctx, zone, header.Name, header.Rrtype)
}

func (s *Server) applyDeleteRecordUpdate(ctx context.Context, zone string, update dns.RR) error {
	record, err := RecordFromRR(zone, update)
	if err != nil {
		return err
	}

	record.Class = dns.ClassINET
	deleteRecordErr := s.deleteRecord(ctx, record)
	if deleteRecordErr != nil {
		return oops.In("dnsserver").
			With("op", "apply_update_delete_record", "zone", zone, "name", record.Name, "type", record.Type).
			Wrapf(deleteRecordErr, "delete record from update")
	}

	return nil
}

func (s *Server) applySaveRecordUpdate(ctx context.Context, zone string, update dns.RR) error {
	record, err := RecordFromRR(zone, update)
	if err != nil {
		return err
	}

	saveRecordErr := s.saveRecord(ctx, record)
	if saveRecordErr != nil {
		return oops.In("dnsserver").
			With("op", "apply_update_save_record", "zone", zone, "name", record.Name, "type", record.Type).
			Wrapf(saveRecordErr, "save record from update")
	}

	return nil
}

func (s *Server) saveRecord(ctx context.Context, record Record) error {
	if s.manager != nil {
		_, err := s.manager.UpsertRecord(ctx, record)
		return err
	}

	return oops.In("dnsserver").
		With("op", "save_record_helper", "zone", record.Zone, "name", record.Name, "type", record.Type).
		Wrapf(s.repo.SaveRecord(ctx, record), "save record with repository")
}

func (s *Server) deleteRecord(ctx context.Context, record Record) error {
	if s.manager != nil {
		return s.manager.DeleteRecord(ctx, record)
	}

	return oops.In("dnsserver").
		With("op", "delete_record_helper", "zone", record.Zone, "name", record.Name, "type", record.Type).
		Wrapf(s.repo.DeleteRecord(ctx, record), "delete record with repository")
}

func (s *Server) deleteName(ctx context.Context, zone, name string) error {
	records, err := s.repo.LookupAll(ctx, zone, name, dns.ClassANY)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "delete_name", "zone", zone, "name", name).
			Wrapf(err, "lookup records for delete name")
	}

	for _, record := range records {
		deleteErr := s.deleteRecord(ctx, record)
		if deleteErr != nil {
			return oops.In("dnsserver").
				With("op", "delete_name_record", "zone", zone, "name", name, "record_name", record.Name, "type", record.Type).
				Wrapf(deleteErr, "delete record for name")
		}
	}

	return nil
}

func (s *Server) deleteRRSet(ctx context.Context, zone, name string, rrtype uint16) error {
	records, err := s.repo.Lookup(ctx, zone, name, rrtype, dns.ClassANY)
	if err != nil {
		return oops.In("dnsserver").
			With("op", "delete_rrset", "zone", zone, "name", name, "type", rrtype).
			Wrapf(err, "lookup rrset for delete")
	}

	for _, record := range records {
		deleteErr := s.deleteRecord(ctx, record)
		if deleteErr != nil {
			return oops.In("dnsserver").
				With("op", "delete_rrset_record", "zone", zone, "name", name, "type", rrtype, "record_name", record.Name).
				Wrapf(deleteErr, "delete record for rrset")
		}
	}

	return nil
}
