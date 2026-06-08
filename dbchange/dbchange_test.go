package dbchange

import (
	"encoding/json"
	"testing"
)

func TestOperationTypeConstants(t *testing.T) {
	if OperationInsert != "INSERT" {
		t.Errorf("OperationInsert = %q, want %q", OperationInsert, "INSERT")
	}
	if OperationUpdate != "UPDATE" {
		t.Errorf("OperationUpdate = %q, want %q", OperationUpdate, "UPDATE")
	}
	if OperationDelete != "DELETE" {
		t.Errorf("OperationDelete = %q, want %q", OperationDelete, "DELETE")
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	version := int64(42)
	companyID := "company-123"
	locationID := "location-456"
	sessionID := "session-789"
	userID := "user-abc"

	original := Event{
		Operation:     OperationInsert,
		ID:            "id-1",
		Table:         "order",
		Key:           []string{"id-1"},
		Version:       &version,
		ModelVersion:  "v1",
		Region:        "us-central1",
		CompanyID:     &companyID,
		LocationID:    &locationID,
		SessionID:     &sessionID,
		UserID:        &userID,
		After:         json.RawMessage(`{"id":"id-1","name":"test"}`),
		Diff:          []string{"name"},
		Timestamp:     1700000000,
		MVCCTimestamp: "1700000000.0000000000",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Operation != OperationInsert {
		t.Errorf("Operation = %q, want %q", decoded.Operation, OperationInsert)
	}
	if decoded.Region != "us-central1" {
		t.Errorf("Region = %q, want %q", decoded.Region, "us-central1")
	}
	if decoded.SessionID == nil || *decoded.SessionID != "session-789" {
		t.Errorf("SessionID = %v, want %q", decoded.SessionID, "session-789")
	}
	if decoded.Version == nil || *decoded.Version != 42 {
		t.Errorf("Version = %v, want 42", decoded.Version)
	}
}

func TestEventJSONVersionNil(t *testing.T) {
	e := Event{
		Operation: OperationUpdate,
		ID:        "id-1",
		Table:     "order",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Version should be omitted when nil
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if _, exists := raw["version"]; exists {
		t.Error("version field should be omitted when nil")
	}
}

func TestEventJSONSessionIDOmitted(t *testing.T) {
	e := Event{
		Operation: OperationInsert,
		ID:        "id-1",
		Table:     "order",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if _, exists := raw["sessionId"]; exists {
		t.Error("sessionId field should be omitted when nil")
	}
}

func TestEventString(t *testing.T) {
	e := Event{
		Operation: OperationDelete,
		ID:        "id-1",
		Table:     "order",
		Key:       []string{"id-1"},
	}
	want := "Event[op=DELETE,table=order,id=id-1,pk=id-1]"
	if got := e.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestEventGetDelete(t *testing.T) {
	e := Event{
		Operation: OperationDelete,
		Before:    json.RawMessage(`{"id":"id-1","name":"test"}`),
	}

	var res map[string]any
	if err := e.Get(&res); err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if res["name"] != "test" {
		t.Errorf("Get() name = %v, want %q", res["name"], "test")
	}
}

func TestEventGetInsert(t *testing.T) {
	e := Event{
		Operation: OperationInsert,
		After:     json.RawMessage(`{"id":"id-1","name":"created"}`),
	}

	var res map[string]any
	if err := e.Get(&res); err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if res["name"] != "created" {
		t.Errorf("Get() name = %v, want %q", res["name"], "created")
	}
}

func TestOperationTypeComparison(t *testing.T) {
	// Verify that string literal comparison still works with OperationType
	e := Event{Operation: OperationDelete}
	if e.Operation != "DELETE" {
		t.Error("OperationType should be comparable with string literals")
	}
}

func TestEventJSONVersionZero(t *testing.T) {
	// Version = &0 should be serialized as "version":0 (not omitted)
	v := int64(0)
	e := Event{
		Operation: OperationInsert,
		ID:        "id-1",
		Table:     "order",
		Version:   &v,
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	versionRaw, exists := raw["version"]
	if !exists {
		t.Fatal("version field should be present when pointer to 0")
	}
	if string(versionRaw) != "0" {
		t.Errorf("version = %s, want 0", string(versionRaw))
	}
}

func TestEventJSONFieldNames(t *testing.T) {
	// Verify JSON field names match the expected format for downstream consumers
	version := int64(1)
	sid := "s1"
	e := Event{
		Operation:     OperationInsert,
		ID:            "id-1",
		Table:         "t",
		Version:       &version,
		Region:        "us",
		SessionID:     &sid,
		MVCCTimestamp: "ts",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	expectedFields := []string{"operation", "id", "table", "version", "modelVersion", "region", "sessionId", "timestamp", "mvccTimestamp"}
	for _, field := range expectedFields {
		if _, exists := raw[field]; !exists {
			t.Errorf("expected JSON field %q to be present", field)
		}
	}
}
