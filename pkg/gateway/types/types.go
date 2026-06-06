package types

import (
	"errors"
	"strings"
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
	req.ConversationID = strings.TrimSpace(req.ConversationID)
	req.Message = strings.TrimSpace(req.Message)
	req.UserID = strings.TrimSpace(req.UserID)
	req.Source = strings.TrimSpace(req.Source)
	req.Instruct = strings.TrimSpace(req.Instruct)
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
		Code:    strings.TrimSpace(code),
		Message: strings.TrimSpace(message),
	}
}
