package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	"perpetual/internal/registration"
)

type Service interface {
	Register(context.Context, registration.Request) registration.Outcome
	InspectRequest(context.Context, registration.RequestID) (registration.Record, bool, error)
	InspectMachine(context.Context, registration.MachineID) (registration.Record, bool, error)
	Health(context.Context) error
}

type Options struct {
	NewMachineID func() (registration.MachineID, error)
	Fatal        func(any)
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
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return registration.MachineID(fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])), nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method not allowed", "")
			return
		}
		if err := s.service.Health(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "service unavailable", "")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
		return
	}
	if raw, ok := pathID(r.URL.Path, "/v1/registrations/"); ok {
		if err := registration.ValidateRequestID(raw); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid request ID", "")
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
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method not allowed", "")
		}
		return
	}
	if raw, ok := pathID(r.URL.Path, "/v1/machines/"); ok {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "invalid_request", "method not allowed", "")
			return
		}
		if err := registration.ValidateMachineID(raw); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid machine ID", "")
			return
		}
		record, found, err := s.service.InspectMachine(r.Context(), registration.MachineID(raw))
		writeInspection(w, record, found, err)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "route not found", "")
}

func pathID(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(path, prefix)
	return raw, raw != "" && !strings.Contains(raw, "/")
}

func (s *Server) put(w http.ResponseWriter, r *http.Request, id registration.RequestID) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, "invalid_request", "content type must be application/json", "")
		return
	}
	parameters, err := DecodeRegistration(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid registration body", "")
		return
	}
	candidate, err := s.newID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "identity generation failed", "")
		return
	}
	request, err := registration.NewRequest(id, candidate, parameters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid registration request", "")
		return
	}
	outcome := s.service.Register(r.Context(), request)
	switch outcome.Kind {
	case registration.OutcomeCreated:
		writeRecord(w, http.StatusCreated, outcome.Record)
	case registration.OutcomeExisting:
		writeRecord(w, http.StatusOK, outcome.Record)
	case registration.OutcomeRequestConflict:
		writeError(w, http.StatusConflict, "request_conflict", "request ID already owns different parameters", "")
	case registration.OutcomeNameConflict:
		writeError(w, http.StatusConflict, "name_conflict", "machine name is already registered", "")
	case registration.OutcomeCapacity:
		writeError(w, http.StatusTooManyRequests, "registration_capacity", "registration capacity reached", "")
	case registration.OutcomeBusy:
		writeError(w, http.StatusServiceUnavailable, "busy", "registration service busy", "")
	case registration.OutcomeUnavailable:
		writeError(w, http.StatusServiceUnavailable, "unavailable", "registration service unavailable", "")
	case registration.OutcomeUnknown:
		writeError(w, http.StatusServiceUnavailable, "outcome_unknown", "registration may have committed; inspect or retry with the same request ID and parameters", string(id))
	default:
		panic(fmt.Sprintf("unexpected registration outcome %d", outcome.Kind))
	}
}

func writeInspection(w http.ResponseWriter, record registration.Record, found bool, err error) {
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "inspection unavailable", "")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "registration not observed", "")
		return
	}
	writeRecord(w, http.StatusOK, record)
}

func writeRecord(w http.ResponseWriter, status int, record registration.Record) {
	writeJSON(w, status, struct {
		RequestID   registration.RequestID  `json:"request_id"`
		MachineID   registration.MachineID  `json:"machine_id"`
		State       string                  `json:"state"`
		Provisioned bool                    `json:"provisioned"`
		Parameters  registration.Parameters `json:"parameters"`
		CreatedAt   string                  `json:"created_at"`
	}{
		RequestID: record.RequestID, MachineID: record.MachineID, State: "registered",
		Provisioned: false, Parameters: record.Parameters, CreatedAt: record.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	})
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id,omitempty"`
		} `json:"error"`
	}{Error: struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	}{Code: code, Message: message, RequestID: requestID}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	if len(encoded)+1 > 16<<10 {
		panic("registration response exceeds bound")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		log.Printf("HTTP response write failed for status %d: %v", status, err)
	}
}

// FatalHandler prevents net/http's normal panic recovery from keeping a
// potentially corrupted service alive. The callback is used by subprocess
// tests; a returning callback still cannot resume request handling.
func FatalHandler(handler http.Handler, fatal func(any)) http.Handler {
	if fatal == nil {
		fatal = func(value any) {
			fmt.Fprintf(os.Stderr, "fatal HTTP panic: %v\n", value)
			os.Exit(1)
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
