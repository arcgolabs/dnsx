package main

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type errorResponse struct {
	Error string `json:"error"`
}

func newAdminHandler(logger *slog.Logger, manager *dnsserver.Manager) http.Handler {
	handler := &adminHandler{
		logger:  lo.Ternary(logger != nil, logger, slog.Default()),
		manager: manager,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.handleHealth)
	mux.HandleFunc("/zones", handler.handleZones)
	mux.HandleFunc("/zones/", handler.handleZone)
	mux.HandleFunc("/records", handler.handleRecords)
	mux.HandleFunc("/seed/import", handler.handleSeedImport)

	return mux
}

type adminHandler struct {
	logger  *slog.Logger
	manager *dnsserver.Manager
}

func (h *adminHandler) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet)
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *adminHandler) handleZones(writer http.ResponseWriter, request *http.Request) {
	if err := h.requireManager(); err != nil {
		h.writeError(writer, http.StatusServiceUnavailable, err)
		return
	}

	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet)
		return
	}

	zones, err := h.manager.ListZones(request.Context())
	if err != nil {
		h.writeError(writer, http.StatusInternalServerError, err)
		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{"zones": zones})
}

func (h *adminHandler) handleZone(writer http.ResponseWriter, request *http.Request) {
	if err := h.requireManager(); err != nil {
		h.writeError(writer, http.StatusServiceUnavailable, err)
		return
	}

	zone := strings.TrimSpace(strings.TrimPrefix(request.URL.Path, "/zones/"))
	if zone == "" {
		writeJSON(writer, http.StatusBadRequest, errorResponse{Error: "zone path is required"})
		return
	}

	switch request.Method {
	case http.MethodPut:
		savedZone, err := h.manager.UpsertZone(request.Context(), dnsserver.Zone{Name: zone})
		if err != nil {
			h.writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, savedZone)
	case http.MethodDelete:
		if err := h.manager.DeleteZone(request.Context(), zone); err != nil {
			h.writeError(writer, http.StatusBadRequest, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"zone": zone, "status": "deleted"})
	default:
		writeMethodNotAllowed(writer, http.MethodPut, http.MethodDelete)
	}
}

func (h *adminHandler) handleRecords(writer http.ResponseWriter, request *http.Request) {
	if err := h.requireManager(); err != nil {
		h.writeError(writer, http.StatusServiceUnavailable, err)
		return
	}

	switch request.Method {
	case http.MethodGet:
		h.handleGetRecords(writer, request)
	case http.MethodPut:
		h.handlePutRecord(writer, request)
	case http.MethodDelete:
		h.handleDeleteRecord(writer, request)
	default:
		writeMethodNotAllowed(writer, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

func (h *adminHandler) handleGetRecords(writer http.ResponseWriter, request *http.Request) {
	filter, err := decodeRecordFilter(request)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	records, err := h.manager.ListRecords(request.Context(), filter)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{"records": records})
}

func (h *adminHandler) handlePutRecord(writer http.ResponseWriter, request *http.Request) {
	record, err := decodeJSONBody[dnsserver.Record](request)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	savedRecord, err := h.manager.UpsertRecord(request.Context(), record)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, savedRecord)
}

func (h *adminHandler) handleDeleteRecord(writer http.ResponseWriter, request *http.Request) {
	record, err := decodeJSONBody[dnsserver.Record](request)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	if err := h.manager.DeleteRecord(request.Context(), record); err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *adminHandler) handleSeedImport(writer http.ResponseWriter, request *http.Request) {
	if err := h.requireManager(); err != nil {
		h.writeError(writer, http.StatusServiceUnavailable, err)
		return
	}

	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, http.MethodPost)
		return
	}

	seed, err := decodeJSONBody[dnsserver.SeedData](request)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	result, err := h.manager.ImportSeedData(request.Context(), seed)
	if err != nil {
		h.writeError(writer, http.StatusBadRequest, err)
		return
	}

	writeJSON(writer, http.StatusOK, result)
}

func (h *adminHandler) writeError(writer http.ResponseWriter, status int, err error) {
	h.logger.Warn("admin request failed", "status", status, "err", err)
	writeJSON(writer, status, errorResponse{Error: err.Error()})
}

func (h *adminHandler) requireManager() error {
	if h == nil || h.manager == nil {
		return oops.In("cmd/server").
			With("op", "require_admin_manager").
			New("dns admin manager is not configured")
	}

	return nil
}
