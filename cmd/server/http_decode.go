package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

func decodeRecordFilter(request *http.Request) (dnsserver.RecordFilter, error) {
	values := request.URL.Query()
	filter := dnsserver.RecordFilter{
		Zone: values.Get("zone"),
		Name: values.Get("name"),
	}

	if rrType, ok, err := parseOptionalRRType(values.Get("type")); err != nil {
		return dnsserver.RecordFilter{}, err
	} else if ok {
		filter.Type = rrType
	}

	if rrClass, ok, err := parseOptionalRRClass(values.Get("class")); err != nil {
		return dnsserver.RecordFilter{}, err
	} else if ok {
		filter.Class = rrClass
	}

	return filter, nil
}

func parseOptionalRRType(value string) (uint16, bool, error) {
	parsed, err := parseOptionalUint16(value, func(normalized string) mo.Option[uint16] {
		if rrType, ok := dns.StringToType[normalized]; ok {
			return mo.Some(rrType)
		}
		return mo.None[uint16]()
	})
	if err != nil {
		return 0, false, oops.In("cmd/server").
			With("op", "parse_rr_type", "value", value).
			Wrapf(err, "parse rr type")
	}

	if parsed.IsAbsent() {
		return 0, false, nil
	}

	rrType, _ := parsed.Get()
	return rrType, true, nil
}

func parseOptionalRRClass(value string) (uint16, bool, error) {
	parsed, err := parseOptionalUint16(value, func(normalized string) mo.Option[uint16] {
		if rrClass, ok := dns.StringToClass[normalized]; ok {
			return mo.Some(rrClass)
		}
		return mo.None[uint16]()
	})
	if err != nil {
		return 0, false, oops.In("cmd/server").
			With("op", "parse_rr_class", "value", value).
			Wrapf(err, "parse rr class")
	}

	if parsed.IsAbsent() {
		return 0, false, nil
	}

	rrClass, _ := parsed.Get()
	return rrClass, true, nil
}

func parseOptionalUint16(value string, named func(string) mo.Option[uint16]) (mo.Option[uint16], error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return mo.None[uint16](), nil
	}

	if option := named(normalized); option.IsPresent() {
		return option, nil
	}

	parsed, err := strconv.ParseUint(normalized, 10, 16)
	if err != nil {
		return mo.None[uint16](), oops.In("cmd/server").
			With("op", "parse_optional_uint16", "value", value).
			Wrapf(err, "parse uint16 value")
	}

	return mo.Some(uint16(parsed)), nil
}

func decodeJSONBody[T any](request *http.Request) (T, error) {
	var zero T

	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		return zero, oops.In("cmd/server").
			With("op", "decode_json_body", "method", request.Method, "path", request.URL.Path).
			Wrapf(err, "read request body")
	}

	defer func() {
		if closeErr := request.Body.Close(); closeErr != nil {
			slog.Default().Warn("close admin request body failed", "err", closeErr)
		}
	}()

	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		return zero, oops.In("cmd/server").
			With("op", "decode_json_body", "method", request.Method, "path", request.URL.Path).
			Wrapf(err, "decode request json")
	}

	return value, nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		slog.Default().Error("write admin json response failed", "err", err)
	}
}

func writeMethodNotAllowed(writer http.ResponseWriter, methods ...string) {
	writer.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSON(writer, http.StatusMethodNotAllowed, errorResponse{
		Error: "method not allowed",
	})
}
