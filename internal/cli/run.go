package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"perpetual/internal/registration"
)

const maxResponseBytes = 16 * 1024

type Options struct {
	Client       *http.Client
	BaseURL      string
	NewRequestID func() (registration.RequestID, error)
}

type registrationResponse struct {
	RequestID   registration.RequestID  `json:"request_id"`
	MachineID   registration.MachineID  `json:"machine_id"`
	State       string                  `json:"state"`
	Provisioned *bool                   `json:"provisioned"`
	Parameters  registration.Parameters `json:"parameters"`
	CreatedAt   time.Time               `json:"created_at"`
}

type errorResponse struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// Run parses one CLI command, performs at most one HTTP request, and returns
// the documented process exit code. It has no process-global I/O or config.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, options Options) int {
	if ctx == nil || stdout == nil || stderr == nil {
		return report(stderr, 5, "CLI requires a context and output streams")
	}
	if len(args) >= 2 && args[0] == "machine" && args[1] == "register" {
		return runRegister(ctx, args[2:], stdout, stderr, options)
	}
	if len(args) == 3 && args[1] == "inspect" && (args[0] == "registration" || args[0] == "machine") {
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
		return report(stderr, 6, "registration result is unknown; inspect or retry request ID %s: %v", requestID, err)
	}
	defer response.Body.Close()
	responseBody, err := readBounded(response.Body)
	if err != nil {
		return report(stderr, 6, "registration result is unknown for request ID %s", requestID)
	}
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
		if err := validateRegistrationResponse(responseBody, requestID, "", &parameters); err != nil {
			return report(stderr, 6, "registration result is unknown; inspect or retry request ID %s: %v", requestID, err)
		}
		if err := writeJSONLine(stdout, responseBody); err != nil {
			return report(stderr, 5, "could not write registration response: %v", err)
		}
		return 0
	}
	return mapErrorResponse(response.StatusCode, responseBody, true, requestID, stderr)
}

func runInspect(ctx context.Context, kind, identifier string, stdout, stderr io.Writer, options Options) int {
	if kind == "registration" {
		if err := registration.ValidateRequestID(identifier); err != nil {
			return report(stderr, 2, "invalid request ID: %v", err)
		}
	} else if err := registration.ValidateMachineID(identifier); err != nil {
		return report(stderr, 2, "invalid machine ID: %v", err)
	}
	plural := kind + "s"
	endpoint, err := endpointURL(options.BaseURL, "/v1/"+plural+"/"+identifier)
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
		var expectedRequest registration.RequestID
		var expectedMachine registration.MachineID
		if kind == "registration" {
			expectedRequest = registration.RequestID(identifier)
		} else {
			expectedMachine = registration.MachineID(identifier)
		}
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
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
	}
	return body, nil
}

func validateRegistrationResponse(body []byte, expectedRequestID registration.RequestID, expectedMachineID registration.MachineID, expectedParameters *registration.Parameters) error {
	var response registrationResponse
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
	var response errorResponse
	decodeErr := json.Unmarshal(body, &response)
	code := response.Error.Code
	// Only a recognized service rejection establishes a known mutation result.
	// A proxy or malformed response cannot prove that the operation did not commit.
	if mutation && (decodeErr != nil || !knownMutationRejection(status, code)) {
		return report(stderr, 6, "registration result is unknown; inspect or retry request ID %s", requestID)
	}
	exitCode := 5
	switch status {
	case http.StatusBadRequest:
		exitCode = 2
	case http.StatusConflict:
		exitCode = 3
	case http.StatusNotFound:
		exitCode = 4
	case http.StatusTooManyRequests:
		exitCode = 5
	case http.StatusServiceUnavailable:
		if mutation && code == "outcome_unknown" {
			exitCode = 6
		}
	}
	if mutation && exitCode == 6 {
		return report(stderr, exitCode, "registration result is unknown; inspect or retry request ID %s", requestID)
	}
	if code == "" {
		return report(stderr, exitCode, "control plane returned HTTP %d", status)
	}
	return report(stderr, exitCode, "control plane returned %s (HTTP %d)", code, status)
}

func knownMutationRejection(status int, code string) bool {
	switch status {
	case http.StatusBadRequest:
		return code == "invalid_request"
	case http.StatusConflict:
		return code == "request_conflict" || code == "name_conflict"
	case http.StatusTooManyRequests:
		return code == "registration_capacity"
	case http.StatusServiceUnavailable:
		return code == "busy" || code == "unavailable"
	default:
		return false
	}
}

func writeJSONLine(writer io.Writer, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return fmt.Errorf("response is not one JSON object")
	}
	if _, err := writer.Write(trimmed); err != nil {
		return err
	}
	_, err := io.WriteString(writer, "\n")
	return err
}

func report(stderr io.Writer, code int, format string, args ...any) int {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, format+"\n", args...)
	}
	return code
}

func randomRequestID() (registration.RequestID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return registration.RequestID(encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]), nil
}
