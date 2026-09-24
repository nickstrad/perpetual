package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"perpetual/internal/httpapi"
	"perpetual/internal/registration"
)

type Options struct {
	Client       *http.Client
	BaseURL      string
	NewRequestID func() (registration.RequestID, error)
}

// Run parses one CLI command, performs at most one HTTP request, and returns
// the documented process exit code. It has no process-global I/O or config.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, options Options) int {
	if stderr == nil {
		return 5 // there is nowhere to report the failure
	}
	if ctx == nil || stdout == nil {
		return report(stderr, 5, "CLI requires a context and output streams")
	}
	if len(args) >= 2 && args[0] == "machine" && args[1] == "register" {
		return runRegister(ctx, args[2:], stdout, stderr, options)
	}
	if len(args) == 3 && args[1] == "inspect" {
		return runInspect(ctx, args[0], args[2], stdout, stderr, options)
	}
	return report(stderr, 2, "invalid command")
}

func runRegister(ctx context.Context, args []string, stdout, stderr io.Writer, options Options) int {
	flags := flag.NewFlagSet("machine register", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	requestIDText := flags.String("request-id", "", "stable registration request ID")
	name := flags.String("name", "", "machine name")
	image := flags.String("image", "", "image name")
	vcpus := flags.Uint64("vcpus", 1, "virtual CPU count")
	memoryMiB := flags.Uint64("memory-mib", 512, "memory in MiB")
	diskMiB := flags.Uint64("disk-mib", 1024, "disk size in MiB")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return report(stderr, 2, "invalid machine register flags")
	}
	if *vcpus > math.MaxUint32 {
		return report(stderr, 2, "vcpus is outside the supported range")
	}
	parameters := registration.Parameters{
		Name:      *name,
		Image:     *image,
		VCPUs:     uint32(*vcpus),
		MemoryMiB: *memoryMiB,
		DiskMiB:   *diskMiB,
	}
	if err := registration.ValidateParameters(parameters); err != nil {
		return report(stderr, 2, "invalid registration parameters: %v", err)
	}

	requestID := registration.RequestID(*requestIDText)
	if requestID == "" {
		newID := options.NewRequestID
		if newID == nil {
			newID = randomRequestID
		}
		var err error
		requestID, err = newID()
		if err != nil {
			return report(stderr, 5, "could not generate request ID: %v", err)
		}
	}
	if err := registration.ValidateRequestID(string(requestID)); err != nil {
		return report(stderr, 2, "invalid request ID: %v", err)
	}
	if _, err := fmt.Fprintf(stderr, "request ID: %s\n", requestID); err != nil {
		return 5
	}
	body, err := json.Marshal(parameters)
	if err != nil {
		return report(stderr, 5, "could not encode registration request")
	}
	endpoint, err := endpointURL(options.BaseURL, "/v1/registrations/"+string(requestID))
	if err != nil {
		return report(stderr, 5, "invalid control-plane URL: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return report(stderr, 5, "could not create registration request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := clientFor(options).Do(request)
	if err != nil {
		return reportUnknown(stderr, requestID, err)
	}
	defer response.Body.Close()
	responseBody, err := readBounded(response.Body)
	if err != nil {
		return reportUnknown(stderr, requestID, err)
	}
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
		if err := validateRegistrationResponse(responseBody, requestID, "", &parameters); err != nil {
			return reportUnknown(stderr, requestID, err)
		}
		if err := writeJSONLine(stdout, responseBody); err != nil {
			return report(stderr, 5, "could not write registration response: %v", err)
		}
		return 0
	}
	return mapErrorResponse(response.StatusCode, responseBody, true, requestID, stderr)
}

func runInspect(ctx context.Context, kind, identifier string, stdout, stderr io.Writer, options Options) int {
	// The kind decides the route, the identifier syntax, and which identity
	// the response must echo. Only that expected identity is set.
	var collection string
	var expectedRequest registration.RequestID
	var expectedMachine registration.MachineID
	switch kind {
	case "registration":
		if err := registration.ValidateRequestID(identifier); err != nil {
			return report(stderr, 2, "invalid request ID: %v", err)
		}
		collection = "registrations"
		expectedRequest = registration.RequestID(identifier)
	case "machine":
		if err := registration.ValidateMachineID(identifier); err != nil {
			return report(stderr, 2, "invalid machine ID: %v", err)
		}
		collection = "machines"
		expectedMachine = registration.MachineID(identifier)
	default:
		return report(stderr, 2, "invalid command")
	}
	endpoint, err := endpointURL(options.BaseURL, "/v1/"+collection+"/"+identifier)
	if err != nil {
		return report(stderr, 5, "invalid control-plane URL: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return report(stderr, 5, "could not create inspection request: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := clientFor(options).Do(request)
	if err != nil {
		return report(stderr, 5, "inspection unavailable: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := readBounded(response.Body)
	if err != nil {
		return report(stderr, 5, "invalid control-plane response: %v", err)
	}
	if response.StatusCode == http.StatusOK {
		if err := validateRegistrationResponse(responseBody, expectedRequest, expectedMachine, nil); err != nil {
			return report(stderr, 5, "invalid control-plane response: %v", err)
		}
		if err := writeJSONLine(stdout, responseBody); err != nil {
			return report(stderr, 5, "could not write inspection response: %v", err)
		}
		return 0
	}
	return mapErrorResponse(response.StatusCode, responseBody, false, "", stderr)
}

func clientFor(options Options) *http.Client {
	var client http.Client
	if options.Client != nil {
		client = *options.Client
	} else {
		client.Timeout = 10 * time.Second
	}
	// A redirect could cause a second mutation request. Return its response as-is.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func endpointURL(baseURL, path string) (string, error) {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:7777"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("must be an absolute HTTP or HTTPS URL without credentials or query")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	return parsed.String(), nil
}

func readBounded(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, httpapi.MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > httpapi.MaxResponseBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", httpapi.MaxResponseBytes)
	}
	return body, nil
}

func validateRegistrationResponse(body []byte, expectedRequestID registration.RequestID, expectedMachineID registration.MachineID, expectedParameters *registration.Parameters) error {
	var response httpapi.RegistrationResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("response is not a registration object")
	}
	if err := registration.ValidateRequestID(string(response.RequestID)); err != nil {
		return fmt.Errorf("response has an invalid request ID")
	}
	if expectedRequestID != "" && response.RequestID != expectedRequestID {
		return fmt.Errorf("response request ID does not match the submitted request")
	}
	if err := registration.ValidateMachineID(string(response.MachineID)); err != nil {
		return fmt.Errorf("response has an invalid machine ID")
	}
	if expectedMachineID != "" && response.MachineID != expectedMachineID {
		return fmt.Errorf("response machine ID does not match the inspected machine")
	}
	if response.State != "registered" || response.Provisioned == nil || *response.Provisioned {
		return fmt.Errorf("response is not an unprovisioned registration")
	}
	if err := registration.ValidateParameters(response.Parameters); err != nil {
		return fmt.Errorf("response has invalid parameters")
	}
	if expectedParameters != nil && response.Parameters != *expectedParameters {
		return fmt.Errorf("response parameters do not match the submitted request")
	}
	if response.CreatedAt.IsZero() {
		return fmt.Errorf("response has no creation timestamp")
	}
	return nil
}

func mapErrorResponse(status int, body []byte, mutation bool, requestID registration.RequestID, stderr io.Writer) int {
	var response httpapi.ErrorResponse
	decodeErr := json.Unmarshal(body, &response)
	code := response.Error.Code
	// Only a recognized service rejection establishes a known mutation result.
	// A proxy or malformed response cannot prove that the operation did not
	// commit, and outcome_unknown says so explicitly.
	if mutation && (decodeErr != nil || !knownMutationRejection(status, code)) {
		return reportUnknown(stderr, requestID, nil)
	}
	exitCode := 5
	switch status {
	case http.StatusBadRequest:
		exitCode = 2
	case http.StatusConflict:
		exitCode = 3
	case http.StatusNotFound:
		exitCode = 4
	}
	if code == "" {
		return report(stderr, exitCode, "control plane returned HTTP %d", status)
	}
	return report(stderr, exitCode, "control plane returned %s (HTTP %d)", code, status)
}

func knownMutationRejection(status int, code string) bool {
	switch status {
	case http.StatusBadRequest:
		return code == httpapi.CodeInvalidRequest
	case http.StatusConflict:
		return code == httpapi.CodeRequestConflict || code == httpapi.CodeNameConflict
	case http.StatusTooManyRequests:
		return code == httpapi.CodeRegistrationCapacity
	case http.StatusServiceUnavailable:
		return code == httpapi.CodeBusy || code == httpapi.CodeUnavailable
	default:
		return false
	}
}

func writeJSONLine(writer io.Writer, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	// Callers already unmarshalled these bytes; this guards the stdout shape.
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("response is not one JSON object")
	}
	if _, err := writer.Write(trimmed); err != nil {
		return err
	}
	_, err := io.WriteString(writer, "\n")
	return err
}

func report(stderr io.Writer, code int, format string, args ...any) int {
	_, _ = fmt.Fprintf(stderr, format+"\n", args...)
	return code
}

// reportUnknown reports a registration whose commit cannot be ruled out. The
// request ID lets the operator inspect or retry the same registration safely.
func reportUnknown(stderr io.Writer, requestID registration.RequestID, cause error) int {
	if cause == nil {
		return report(stderr, 6, "registration result is unknown; inspect or retry request ID %s", requestID)
	}
	return report(stderr, 6, "registration result is unknown; inspect or retry request ID %s: %v", requestID, cause)
}

func randomRequestID() (registration.RequestID, error) {
	id, err := registration.NewRandomID()
	return registration.RequestID(id), err
}
