package v1

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
)

const ProtocolVersion = 1

type Envelope struct {
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     *Error          `json:"error,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

type Status struct {
	ProtocolVersion        int      `json:"protocol_version"`
	HostVersion            string   `json:"host_version"`
	ActiveProfileID        string   `json:"active_profile_id"`
	MonitorState           string   `json:"monitor_state"`
	NotificationPermission string   `json:"notification_permission"`
	Coverage               Coverage `json:"coverage"`
}

type Coverage struct {
	State string `json:"state"`
}

type RuleValidation struct {
	Summary        string   `json:"summary"`
	RequiredFields []string `json:"required_fields"`
}

type SchemaError struct {
	Path    string
	Message string
}

func (e *SchemaError) Error() string { return e.Path + ": " + e.Message }

func DecodeEnvelope(data []byte) (Envelope, error) {
	object, err := DecodeObject(data, "envelope", []string{"request_id", "data", "error", "meta"})
	if err != nil {
		return Envelope{}, err
	}
	var result Envelope
	if err := DecodeValue(object["request_id"], "request_id", &result.RequestID); err != nil {
		return Envelope{}, err
	}
	id, err := uuid.Parse(result.RequestID)
	if err != nil || id == uuid.Nil || id.String() != result.RequestID {
		return Envelope{}, schema("request_id", "must be a canonical nonzero UUID")
	}
	_, hasData := object["data"]
	_, hasError := object["error"]
	if hasData == hasError {
		return Envelope{}, schema("envelope", "must contain exactly one of data or error")
	}
	if hasData {
		if _, err := DecodeObject(object["data"], "data", nil); err != nil {
			return Envelope{}, err
		}
		result.Data = object["data"]
	} else {
		fields, err := DecodeObject(object["error"], "error", []string{"code", "message", "retryable", "details"})
		if err != nil {
			return Envelope{}, err
		}
		failure := &Error{}
		if err := DecodeValue(fields["code"], "error.code", &failure.Code); err != nil {
			return Envelope{}, err
		}
		if err := DecodeValue(fields["message"], "error.message", &failure.Message); err != nil {
			return Envelope{}, err
		}
		if failure.Code == "" || failure.Message == "" {
			return Envelope{}, schema("error", "code and message must be nonempty")
		}
		if err := DecodeValue(fields["retryable"], "error.retryable", &failure.Retryable); err != nil {
			return Envelope{}, err
		}
		if _, err := DecodeObject(fields["details"], "error.details", nil); err != nil {
			return Envelope{}, err
		}
		if err := DecodeValue(fields["details"], "error.details", &failure.Details); err != nil {
			return Envelope{}, err
		}
		result.Error = failure
	}
	if meta, present := object["meta"]; present {
		if _, err := DecodeObject(meta, "meta", nil); err != nil {
			return Envelope{}, err
		}
		result.Meta = meta
	}
	return result, nil
}

// A nil allowed list permits an extensible object; an explicit list is strict.
func DecodeObject(data []byte, path string, allowed []string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(data) {
		return nil, schema(path, "must be valid UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, schema(path, "must be a JSON object")
	}
	result := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, schema(path, "malformed JSON object")
		}
		key, ok := token.(string)
		if !ok {
			return nil, schema(path, "malformed JSON property")
		}
		if _, duplicate := result[key]; duplicate {
			return nil, schema(path, "duplicate properties are not allowed")
		}
		if allowed != nil && !slices.Contains(allowed, key) {
			return nil, schema(path, "unknown properties are not allowed")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, schema(path, "malformed JSON value")
		}
		result[key] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, schema(path, "malformed JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, schema(path, "must contain only one JSON value")
	}
	return result, nil
}

func DecodeValue(data []byte, path string, target any) error {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) ||
		!utf8.Valid(data) || json.Unmarshal(data, target) != nil {
		return schema(path, "missing value or incompatible JSON type")
	}
	return nil
}

func schema(path, message string) error { return &SchemaError{Path: path, Message: message} }
