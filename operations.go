package dock

import (
	"context"
	"encoding/json"
	"net/url"
)

// Operation is a long-running change, such as a delete.
type Operation struct {
	ID         string `json:"id"`
	TargetType string `json:"targetType,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	Status     string `json:"status"`
}

// OperationsService reads operations. Get it from Client.Operations.
type OperationsService struct{ c *Client }

// Get returns one operation.
func (s *OperationsService) Get(ctx context.Context, operationID string) (*Operation, error) {
	var raw json.RawMessage
	if err := s.c.do(ctx, "GET", "/operations/"+url.PathEscape(operationID), nil, nil, nil, &raw); err != nil {
		return nil, err
	}
	// Accept the bare object or one wrapped as {"operation": {...}}.
	var wrapped struct {
		Operation *Operation `json:"operation"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Operation != nil {
		return wrapped.Operation, nil
	}
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, &Error{Code: "bad_response", Message: "unexpected operation shape"}
	}
	return &op, nil
}
