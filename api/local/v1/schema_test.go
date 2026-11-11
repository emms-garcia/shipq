package v1_test

import (
	"encoding/json"
	"os"
	"testing"

	v1 "github.com/emms-garcia/shipq/api/local/v1"
)

func TestSharedEnvelopeFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../tests/fixtures/protocol/base_envelopes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Valid []struct {
			ID       string          `json:"id"`
			Envelope json.RawMessage `json:"envelope"`
		} `json:"valid"`
		Invalid []struct {
			ID       string          `json:"id"`
			Envelope json.RawMessage `json:"envelope"`
		} `json:"invalid"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures.Valid {
		t.Run(f.ID, func(t *testing.T) {
			if _, err := v1.DecodeEnvelope(f.Envelope); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, f := range fixtures.Invalid {
		t.Run(f.ID, func(t *testing.T) {
			if _, err := v1.DecodeEnvelope(f.Envelope); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
}

func TestStrictEnvelopeAndObjectSchema(t *testing.T) {
	for _, data := range []string{
		`null`, `[]`, `{"request_id":"not-a-uuid","data":{}}`,
		`{"request_id":"00000000-0000-0000-0000-000000000000","data":{}}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","data":null}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","data":{},"unknown":true}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","data":{},"data":{}}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","error":{"code":"internal","message":"failure","details":{}}}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","error":{"code":"internal","message":"failure","retryable":null,"details":{}}}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","error":{"code":"internal","message":"failure","retryable":false,"details":[]}}`,
		`{"request_id":"11111111-1111-4111-8111-111111111111","data":{}} {}`,
	} {
		if _, err := v1.DecodeEnvelope([]byte(data)); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
	for _, data := range []string{`{"a":1,"a":2}`, `{"a":1,"other":2}`, `{"a":1} trailing`, "\xff", `{"a":`} {
		if _, err := v1.DecodeObject([]byte(data), "test", []string{"a"}); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
	var value bool
	for _, data := range []string{`null`, `"true"`, `true false`} {
		if err := v1.DecodeValue([]byte(data), "value", &value); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}
