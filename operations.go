package dock

import (
	"context"
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
	var op Operation
	if err := s.c.do(ctx, "GET", "/operations/"+url.PathEscape(operationID), nil, nil, nil, &op); err != nil {
		return nil, err
	}
	return &op, nil
}
