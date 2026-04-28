package dnsserver

import (
	"errors"

	"github.com/samber/oops"
)

type ErrorCode string

const (
	CodeRepositoryNotConfigured ErrorCode = "repository_not_configured"
	CodeZoneNameRequired        ErrorCode = "zone_name_required"
	CodeRecordNameRequired      ErrorCode = "record_name_required"
	CodeRecordDataRequired      ErrorCode = "record_data_required"
	CodeRecordTypeRequired      ErrorCode = "record_type_required"
	CodeRecordOutOfZone         ErrorCode = "record_out_of_zone"
	CodeRRSetNameRequired       ErrorCode = "rrset_name_required"
	CodeRRSetTypeRequired       ErrorCode = "rrset_type_required"
	CodeRRSetRecordsRequired    ErrorCode = "rrset_records_required"
	CodeRRSetMismatch           ErrorCode = "rrset_mismatch"
	CodeServerNil               ErrorCode = "server_nil"
	CodeHandlerNil              ErrorCode = "handler_nil"
	CodeServerAlreadyStarted    ErrorCode = "server_already_started"
)

var (
	ErrRepositoryNotConfigured = errors.New("dns repository is not configured")
	ErrZoneNameRequired        = errors.New("zone name is required")
	ErrRecordNameRequired      = errors.New("record name is required")
	ErrRecordDataRequired      = errors.New("record data is required")
	ErrRecordTypeRequired      = errors.New("record type is required")
	ErrRecordOutOfZone         = errors.New("record is outside zone")
	ErrRRSetNameRequired       = errors.New("rrset name is required")
	ErrRRSetTypeRequired       = errors.New("rrset type is required")
	ErrRRSetRecordsRequired    = errors.New("rrset records are required")
	ErrRRSetMismatch           = errors.New("rrset record does not match target rrset")
	ErrServerNil               = errors.New("dns server is nil")
	ErrHandlerNil              = errors.New("dns handler is nil")
	ErrServerAlreadyStarted    = errors.New("dns server already started")
)

func errorBuilder(op string, code ErrorCode, kv ...any) oops.OopsErrorBuilder {
	fields := append([]any{"op", op}, kv...)
	return oops.Code(code).In("dnsserver").With(fields...)
}

func ErrorCodeOf(err error) (ErrorCode, bool) {
	oopsErr, ok := oops.AsOops(err)
	if !ok {
		return "", false
	}

	switch code := oopsErr.Code().(type) {
	case ErrorCode:
		return code, true
	case string:
		return ErrorCode(code), true
	default:
		return "", false
	}
}

func HasErrorCode(err error, code ErrorCode) bool {
	actual, ok := ErrorCodeOf(err)
	return ok && actual == code
}
