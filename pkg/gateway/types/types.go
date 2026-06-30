package types

import (
	"errors"

	"github.com/wandxy/morph/pkg/stringx"
)

const (
	ErrorCodeBadRequest    = "bad_request"
	ErrorCodeUnauthorized  = "unauthorized"
	ErrorCodeInternalError = "internal_error"
	SourceGenericHTTP      = "generic_http"
)

var (
	ErrConversationIDRequired = errors.New("conversation_id is required")
	ErrMessageRequired        = errors.New("message is required")
)

type RespondRequest struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
	UserID         string `json:"user_id,omitempty"`
	Source         string `json:"source,omitempty"`
	Instruct       string `json:"instruct,omitempty"`
}

type RespondResponse struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	SessionID      string         `json:"session_id,omitempty"`
	Text           string         `json:"text,omitempty"`
	Error          *ErrorResponse `json:"error,omitempty"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NormalizeRespondRequest(req RespondRequest) RespondRequest {
	req.ConversationID = stringx.String(req.ConversationID).Trim()
	req.Message = stringx.String(req.Message).Trim()
	req.UserID = stringx.String(req.UserID).Trim()
	req.Source = stringx.String(req.Source).Trim()
	req.Instruct = stringx.String(req.Instruct).Trim()
	if req.Source == "" {
		req.Source = SourceGenericHTTP
	}

	return req
}

func ValidateRespondRequest(req RespondRequest) error {
	req = NormalizeRespondRequest(req)
	if req.ConversationID == "" {
		return ErrConversationIDRequired
	}
	if req.Message == "" {
		return ErrMessageRequired
	}

	return nil
}

func NewErrorResponse(code string, message string) ErrorResponse {
	return ErrorResponse{
		Code:    stringx.String(code).Trim(),
		Message: stringx.String(message).Trim(),
	}
}
