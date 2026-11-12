package dto

import (
	"encoding/json"
	"testing"
)

func TestUpdateActionItemRequest_ClearableFields(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantPresent bool
		wantValue   *string
	}{
		{"absent key leaves field untouched", `{}`, false, nil},
		{"explicit null clears the field", `{"assignedTo":null}`, true, nil},
		{"string sets the field", `{"assignedTo":"u1"}`, true, strPtr("u1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req UpdateActionItemRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.AssignedTo.Present != tc.wantPresent {
				t.Fatalf("Present = %v, want %v", req.AssignedTo.Present, tc.wantPresent)
			}
			if (req.AssignedTo.Value == nil) != (tc.wantValue == nil) ||
				(tc.wantValue != nil && *req.AssignedTo.Value != *tc.wantValue) {
				t.Fatalf("Value = %v, want %v", req.AssignedTo.Value, tc.wantValue)
			}
		})
	}
}

func TestUpdateActionItemRequest_AllClearableKeysIndependent(t *testing.T) {
	var req UpdateActionItemRequest
	if err := json.Unmarshal([]byte(`{"dimensionId":null,"dueDate":"2026-05-04"}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.DimensionID.Present || req.DimensionID.Value != nil {
		t.Errorf("dimensionId should be present and nil, got %+v", req.DimensionID)
	}
	if !req.DueDate.Present || req.DueDate.Value == nil || *req.DueDate.Value != "2026-05-04" {
		t.Errorf("dueDate should be set, got %+v", req.DueDate)
	}
	if req.AssignedTo.Present {
		t.Errorf("assignedTo was not sent, must be absent")
	}
}

func TestClearableString_RejectsNonString(t *testing.T) {
	var req UpdateActionItemRequest
	if err := json.Unmarshal([]byte(`{"assignedTo":5}`), &req); err == nil {
		t.Fatal("expected error for non-string assignedTo")
	}
}

func strPtr(s string) *string { return &s }
