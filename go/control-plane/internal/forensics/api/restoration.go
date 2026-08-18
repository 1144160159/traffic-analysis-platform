package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/restoration"
)

type RestorationProcessor interface {
	Process(context.Context, restoration.ProcessRequest) (*restoration.CommitReceipt, error)
}

type restorationRequest struct {
	IdempotencyKey string                      `json:"idempotency_key"`
	SessionID      string                      `json:"session_id"`
	CommunityID    string                      `json:"community_id"`
	FlowIDs        []string                    `json:"flow_ids"`
	FlowID         string                      `json:"flow_id"`
	Tuple          restoration.FiveTuple       `json:"five_tuple"`
	Direction      string                      `json:"direction"`
	StartTime      time.Time                   `json:"capture_time_start"`
	EndTime        time.Time                   `json:"capture_time_end"`
	ProfileID      string                      `json:"protocol_profile_id"`
	FTPData        *restoration.FTPDataRequest `json:"ftp_data,omitempty"`
	FTPTLSEnabled  bool                        `json:"ftp_tls_enabled"`
	Reason         string                      `json:"reason"`
}

func (h *Handler) SetRestorationProcessor(processor RestorationProcessor) {
	h.restorationProcessor = processor
}

func decodeRestorationRequest(w http.ResponseWriter, r *http.Request) (restorationRequest, error) {
	var request restorationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return restorationRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return restorationRequest{}, errors.New("request body must contain exactly one JSON object")
	}
	return request, nil
}

func (h *Handler) CreateRestoration(w http.ResponseWriter, r *http.Request) {
	rw := httpx.NewResponseWriter(w, r.Context())
	if h.restorationProcessor == nil {
		rw.Error(http.StatusServiceUnavailable, "RESTORATION_DISABLED", "file restoration admission is disabled", nil)
		return
	}
	request, err := decodeRestorationRequest(w, r)
	if err != nil {
		rw.Error(http.StatusBadRequest, "INVALID_RESTORATION_REQUEST", "request must be one strict bounded JSON object", nil)
		return
	}
	tenantID := strings.TrimSpace(httpx.GetTenantID(r.Context()))
	actorID := strings.TrimSpace(httpx.GetUserID(r.Context()))
	traceID := strings.TrimSpace(httpx.GetTraceID(r.Context()))
	if traceID == "" {
		traceID = strings.TrimSpace(httpx.GetRequestID(r.Context()))
	}
	if tenantID == "" || actorID == "" || traceID == "" {
		rw.Error(http.StatusUnauthorized, "MISSING_RESTORATION_AUTHORITY", "tenant, actor and trace authority are required", nil)
		return
	}
	receipt, err := h.restorationProcessor.Process(r.Context(), restoration.ProcessRequest{
		TenantID: tenantID, IdempotencyKey: request.IdempotencyKey, SessionID: request.SessionID,
		CommunityID: request.CommunityID, FlowIDs: request.FlowIDs, FlowID: request.FlowID,
		Tuple: request.Tuple, Direction: request.Direction, StartTime: request.StartTime, EndTime: request.EndTime,
		ProfileID: request.ProfileID, FTPData: request.FTPData, FTPTLSEnabled: request.FTPTLSEnabled,
		ActorID: actorID, Reason: request.Reason, TraceID: traceID,
	})
	if err != nil {
		switch {
		case errors.Is(err, restoration.ErrAdmissionDisabled):
			rw.Error(http.StatusServiceUnavailable, "RESTORATION_DISABLED", err.Error(), nil)
		case errors.Is(err, restoration.ErrProcessorDraining):
			rw.Error(http.StatusServiceUnavailable, "RESTORATION_DRAINING", err.Error(), nil)
		case errors.Is(err, restoration.ErrTenantConcurrencyExceeded):
			rw.Error(http.StatusTooManyRequests, "RESTORATION_TENANT_CONCURRENCY", err.Error(), nil)
		case errors.Is(err, restoration.ErrRequestInProgress):
			rw.Error(http.StatusConflict, "RESTORATION_IN_PROGRESS", err.Error(), nil)
		case errors.Is(err, restoration.ErrIdempotencyConflict):
			rw.Error(http.StatusConflict, "RESTORATION_IDEMPOTENCY_CONFLICT", err.Error(), nil)
		default:
			rw.Error(http.StatusUnprocessableEntity, "RESTORATION_REJECTED", err.Error(), nil)
		}
		return
	}
	if receipt.Replayed {
		rw.Success(receipt)
		return
	}
	rw.Created(receipt)
}
