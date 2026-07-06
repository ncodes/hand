package types

import (
	"errors"

	"github.com/wandxy/morph/pkg/str"
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
	stringValue1 := str.String(req.ConversationID)
	req.ConversationID = stringValue1.Trim()
	stringValue2 := str.String(req.Message)
	req.Message = stringValue2.Trim()
	stringValue3 := str.String(req.UserID)
	req.UserID = stringValue3.Trim()
	stringValue4 := str.String(req.Source)
	req.Source = stringValue4.Trim()
	stringValue5 := str.String(req.Instruct)
	req.Instruct = stringValue5.Trim()
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
	stringValue6 := str.String(code)
	stringValue7 := str.String(message)
	return ErrorResponse{
		Code:    stringValue6.Trim(),
		Message: stringValue7.Trim(),
	}
}
