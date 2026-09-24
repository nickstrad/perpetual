package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"perpetual/internal/registration"
)

// The wire schema below is shared with the CLI, which decodes these same types.

// MaxResponseBytes bounds every response body, including its trailing newline.
const MaxResponseBytes = 16 << 10

// Error codes carried in ErrorBody.Code.
const (
	CodeInvalidRequest       = "invalid_request"
	CodeRequestConflict      = "request_conflict"
	CodeNameConflict         = "name_conflict"
	CodeRegistrationCapacity = "registration_capacity"
	CodeBusy                 = "busy"
	CodeUnavailable          = "unavailable"
	CodeOutcomeUnknown       = "outcome_unknown"
	CodeNotFound             = "not_found"
)

// RegistrationResponse is the body of a successful registration or inspection.
// CreatedAt marshals as RFC 3339 with trimmed nanoseconds (time.RFC3339Nano).
type RegistrationResponse struct {
	RequestID registration.RequestID `json:"request_id"`
	MachineID registration.MachineID `json:"machine_id"`
	State     string                 `json:"state"`
	// Provisioned is a pointer so a client can reject a missing or null field
	// instead of reading it as false.
	Provisioned *bool                   `json:"provisioned"`
	Parameters  registration.Parameters `json:"parameters"`
	CreatedAt   time.Time               `json:"created_at"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RequestID is set only for outcome_unknown, so the caller can inspect or retry.
	RequestID string `json:"request_id,omitempty"`
}

type Service interface {
	Register(context.Context, registration.Request) registration.Outcome
	InspectRequest(context.Context, registration.RequestID) (registration.Record, bool, error)
	InspectMachine(context.Context, registration.MachineID) (registration.Record, bool, error)
	Health(context.Context) error
}

type Options struct {
	NewMachineID func() (registration.MachineID, error)
}

type Server struct {
	service Service
	newID   func() (registration.MachineID, error)
}

func New(service Service, options Options) *Server {
	newID := options.NewMachineID
	if newID == nil {
		newID = randomMachineID
	}
	return &Server{service: service, newID: newID}
}

func randomMachineID() (registration.MachineID, error) {
	id, err := registration.NewRandomID()
	return registration.MachineID(id), err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if err := s.service.Health(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "service unavailable", "")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
		return
	}
	if raw, ok := pathID(r.URL.Path, "/v1/registrations/"); ok {
		if err := registration.ValidateRequestID(raw); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid request ID", "")
			return
		}
		id := registration.RequestID(raw)
		switch r.Method {
		case http.MethodPut:
			s.put(w, r, id)
		case http.MethodGet:
			record, found, err := s.service.InspectRequest(r.Context(), id)
			writeInspection(w, record, found, err)
		default:
			methodNotAllowed(w)
		}
		return
	}
	if raw, ok := pathID(r.URL.Path, "/v1/machines/"); ok {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if err := registration.ValidateMachineID(raw); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid machine ID", "")
			return
		}
		record, found, err := s.service.InspectMachine(r.Context(), registration.MachineID(raw))
		writeInspection(w, record, found, err)
		return
	}
	writeError(w, http.StatusNotFound, CodeNotFound, "route not found", "")
}

func pathID(path, prefix string) (string, bool) {
	raw, found := strings.CutPrefix(path, prefix)
	if !found {
		return "", false
	}
	return raw, raw != "" && !strings.Contains(raw, "/")
}

// put decodes the body syntactically; registration.NewRequest is the single
// semantic validator for identifiers and parameters.
func (s *Server) put(w http.ResponseWriter, r *http.Request, id registration.RequestID) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "content type must be application/json", "")
		return
	}
	parameters, err := DecodeRegistration(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid registration body", "")
		return
	}
	candidate, err := s.newID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "identity generation failed", "")
		return
	}
	request, err := registration.NewRequest(id, candidate, parameters)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidRequest, "invalid registration request", "")
		return
	}
	outcome := s.service.Register(r.Context(), request)
	switch outcome.Kind {
	case registration.OutcomeCreated:
		writeRecord(w, http.StatusCreated, outcome.Record)
	case registration.OutcomeExisting:
		writeRecord(w, http.StatusOK, outcome.Record)
	case registration.OutcomeRequestConflict:
		writeError(w, http.StatusConflict, CodeRequestConflict, "request ID already owns different parameters", "")
	case registration.OutcomeNameConflict:
		writeError(w, http.StatusConflict, CodeNameConflict, "machine name is already registered", "")
	case registration.OutcomeCapacity:
		writeError(w, http.StatusTooManyRequests, CodeRegistrationCapacity, "registration capacity reached", "")
	case registration.OutcomeBusy:
		writeError(w, http.StatusServiceUnavailable, CodeBusy, "registration service busy", "")
	case registration.OutcomeUnavailable:
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "registration service unavailable", "")
	case registration.OutcomeUnknown:
		writeError(w, http.StatusServiceUnavailable, CodeOutcomeUnknown, "registration may have committed; inspect or retry with the same request ID and parameters", string(id))
	default:
		panic(fmt.Sprintf("unexpected registration outcome %d", outcome.Kind))
	}
}

func writeInspection(w http.ResponseWriter, record registration.Record, found bool, err error) {
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "inspection unavailable", "")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, CodeNotFound, "registration not observed", "")
		return
	}
	writeRecord(w, http.StatusOK, record)
}

func writeRecord(w http.ResponseWriter, status int, record registration.Record) {
	provisioned := false // this slice registers intent only; nothing is provisioned yet
	writeJSON(w, status, RegistrationResponse{
		RequestID:   record.RequestID,
		MachineID:   record.MachineID,
		State:       "registered",
		Provisioned: &provisioned,
		Parameters:  record.Parameters,
		CreatedAt:   record.CreatedAt,
	})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, CodeInvalidRequest, "method not allowed", "")
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message, RequestID: requestID}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	if len(encoded)+1 > MaxResponseBytes {
		panic("registration response exceeds bound")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		log.Printf("HTTP response write failed for status %d: %v", status, err)
	}
}

// FatalHandler prevents net/http's normal panic recovery from keeping a
// potentially corrupted service alive. The callback reports the panic and is
// replaceable by subprocess tests; the process exits after it regardless.
func FatalHandler(handler http.Handler, fatal func(any)) http.Handler {
	if fatal == nil {
		fatal = func(value any) {
			fmt.Fprintf(os.Stderr, "fatal HTTP panic: %v\n", value)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				fatal(value)
				os.Exit(1)
			}
		}()
		handler.ServeHTTP(w, r)
	})
}
