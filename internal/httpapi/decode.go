package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"perpetual/internal/registration"
)

const maxRequestBody = 4 << 10

// DecodeRegistration accepts only the five required fields. Tokenizing keys
// catches duplicates after JSON escape processing (for example "name" and
// "\u006eame"), which unmarshalling straight into a struct would accept.
func DecodeRegistration(body io.Reader) (registration.Parameters, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxRequestBody+1))
	if err != nil {
		return registration.Parameters{}, err
	}
	if len(data) > maxRequestBody {
		return registration.Parameters{}, errors.New("registration body too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return registration.Parameters{}, errors.New("registration body must be an object")
	}
	values := make(map[string]json.RawMessage, 5)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return registration.Parameters{}, err
		}
		key, ok := token.(string)
		if !ok {
			return registration.Parameters{}, errors.New("registration field name invalid")
		}
		switch key {
		case "name", "image", "vcpus", "memory_mib", "disk_mib":
		default:
			return registration.Parameters{}, fmt.Errorf("unknown registration field %q", key)
		}
		if _, duplicate := values[key]; duplicate {
			return registration.Parameters{}, fmt.Errorf("duplicate registration field %q", key)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return registration.Parameters{}, err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return registration.Parameters{}, fmt.Errorf("null registration field %q", key)
		}
		values[key] = raw
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return registration.Parameters{}, errors.New("registration object not closed")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return registration.Parameters{}, errors.New("trailing registration value")
	}
	for _, name := range []string{"name", "image", "vcpus", "memory_mib", "disk_mib"} {
		if _, present := values[name]; !present {
			return registration.Parameters{}, fmt.Errorf("missing registration field %q", name)
		}
	}
	var p registration.Parameters
	for _, field := range []struct {
		name string
		dst  any
	}{
		{"name", &p.Name}, {"image", &p.Image}, {"vcpus", &p.VCPUs},
		{"memory_mib", &p.MemoryMiB}, {"disk_mib", &p.DiskMiB},
	} {
		if err := json.Unmarshal(values[field.name], field.dst); err != nil {
			return registration.Parameters{}, fmt.Errorf("%s: %w", field.name, err)
		}
	}
	if err := registration.ValidateParameters(p); err != nil {
		return registration.Parameters{}, err
	}
	return p, nil
}
