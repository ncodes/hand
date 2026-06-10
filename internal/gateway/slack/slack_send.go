package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	slack "github.com/wandxy/hand/pkg/gateway/slack"
)

const defaultSlackAPIBase = "https://slack.com/api"

type API interface {
	PostMessage(context.Context, slack.Target, string) (string, error)
	StartStream(context.Context, slack.Target, string) (slack.Stream, error)
	AppendStream(context.Context, slack.Stream, string) error
	StopStream(context.Context, slack.Stream, string) error
}

type HTTPClient struct {
	client  *http.Client
	baseURL string
	token   string
}

func NewHTTPClient(token string) *HTTPClient {
	return &HTTPClient{client: http.DefaultClient, baseURL: defaultSlackAPIBase, token: strings.TrimSpace(token)}
}

func (c *HTTPClient) PostMessage(ctx context.Context, target slack.Target, text string) (string, error) {
	var result struct {
		TS string `json:"ts"`
	}
	if err := c.call(ctx, "chat.postMessage", slack.PostMessageRequest{
		Channel:  target.ChannelID,
		ThreadTS: target.ThreadTS,
		Text:     text,
	}, &result); err != nil {
		return "", err
	}

	return result.TS, nil
}

func (c *HTTPClient) StartStream(ctx context.Context, target slack.Target, text string) (slack.Stream, error) {
	var result struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	req := slack.StartStreamRequest{
		Channel:      target.ChannelID,
		ThreadTS:     target.ThreadTS,
		MarkdownText: text,
	}
	if strings.TrimSpace(target.ChannelType) == "im" {
		req.RecipientUserID = target.RecipientUserID
		req.RecipientTeamID = target.RecipientTeamID
	}

	if err := c.call(ctx, "chat.startStream", req, &result); err != nil {
		return slack.Stream{}, err
	}
	if result.Channel == "" {
		result.Channel = target.ChannelID
	}

	return slack.Stream{ChannelID: result.Channel, TS: result.TS}, nil
}

func (c *HTTPClient) AppendStream(ctx context.Context, stream slack.Stream, text string) error {
	return c.call(ctx, "chat.appendStream", slack.AppendStreamRequest{
		Channel:      stream.ChannelID,
		TS:           stream.TS,
		MarkdownText: text,
	}, nil)
}

func (c *HTTPClient) StopStream(ctx context.Context, stream slack.Stream, text string) error {
	return c.call(ctx, "chat.stopStream", slack.StopStreamRequest{
		Channel:      stream.ChannelID,
		TS:           stream.TS,
		MarkdownText: text,
	}, nil)
}

func (c *HTTPClient) call(ctx context.Context, method string, req any, out any) error {
	if c == nil {
		return errors.New("slack client is required")
	}
	if c.client == nil {
		c.client = http.DefaultClient
	}
	baseURL := strings.TrimRight(c.baseURL, "/")
	if baseURL == "" {
		baseURL = defaultSlackAPIBase
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.token))
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return err
	}
	if httpResp.StatusCode == http.StatusTooManyRequests {
		return errors.New("slack api rate limited")
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("slack api http status %d", httpResp.StatusCode)
	}
	if !apiResp.OK {
		if apiResp.Error == "" {
			apiResp.Error = "slack api returned ok=false"
		}
		return errors.New(apiResp.Error)
	}
	if out != nil {
		return json.Unmarshal(respBody, out)
	}

	return nil
}
