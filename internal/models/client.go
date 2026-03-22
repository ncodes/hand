package models

import "context"

type Client interface {
	Chat(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}

type GenerateRequest struct {
	Model           string
	Input           string
	Instructions    string
	MaxOutputTokens int64
	Temperature     float64
}

type GenerateResponse struct {
	ID         string
	Model      string
	OutputText string
}
