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

// registrationField names one required wire field and where its value lands.
type registrationField struct {
	name string
	dst  any // pointer into the Parameters being decoded
}

// registrationFields lists the five required fields in wire order. It is the
// single source for the allow-list, the presence check, and unmarshalling.
func registrationFields(p *registration.Parameters) [5]registrationField {
	return [5]registrationField{
		{"name", &p.Name},
		{"image", &p.Image},
		{"vcpus", &p.VCPUs},
		{"memory_mib", &p.MemoryMiB},
		{"disk_mib", &p.DiskMiB},
	}
}

// DecodeRegistration checks only the body's shape: size, one object, exactly
// the five known fields, no null or duplicate fields, and Go-typed values that
// fit their integer widths. It does not check domain rules such as name syntax
// or resource ranges; registration.NewRequest owns that validation.
// Tokenizing keys catches duplicates after JSON escape processing (for example
// "name" and "\u006eame"), which unmarshalling straight into a struct would accept.
func DecodeRegistration(body io.Reader) (registration.Parameters, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxRequestBody+1))
	if err != nil {
		return registration.Parameters{}, err
	}
	if len(data) > maxRequestBody {
		return registration.Parameters{}, errors.New("registration body too large")
	}
	var p registration.Parameters
	fields := registrationFields(&p)
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return registration.Parameters{}, errors.New("registration body must be an object")
	}
	values := make(map[string]json.RawMessage, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return registration.Parameters{}, err
		}
		key, ok := token.(string)
		if !ok {
			return registration.Parameters{}, errors.New("registration field name invalid")
		}
		if !knownField(fields[:], key) {
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
	for _, field := range fields {
		raw, present := values[field.name]
		if !present {
			return registration.Parameters{}, fmt.Errorf("missing registration field %q", field.name)
		}
		if err := json.Unmarshal(raw, field.dst); err != nil {
			return registration.Parameters{}, fmt.Errorf("%s: %w", field.name, err)
		}
	}
	return p, nil
}

func knownField(fields []registrationField, key string) bool {
	for _, field := range fields {
		if field.name == key {
			return true
		}
	}
	return false
}
