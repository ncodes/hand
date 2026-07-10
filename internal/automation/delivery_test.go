package automation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type deliveryHTTPClientFunc func(*http.Request) (*http.Response, error)

func (fn deliveryHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestDeliverySinkFunc(t *testing.T) {
	called := false
	err := DeliverySinkFunc(func(context.Context, DeliveryRequest) error {
		called = true
		return nil
	}).DeliverAutomation(context.Background(), DeliveryRequest{})
	require.NoError(t, err)
	require.True(t, called)

	err = DeliverySinkFunc(nil).DeliverAutomation(context.Background(), DeliveryRequest{})
	require.EqualError(t, err, "automation delivery sink is required")
}

func TestNormalizeDelivery(t *testing.T) {
	delivery := normalizeDelivery(Delivery{
		Mode:            " Gateway ",
		Channel:         " Slack ",
		Target:          " C1 ",
		ThreadID:        " 123 ",
		WebhookURL:      " https://example.com/hook ",
		FailureTarget:   " ops ",
		FailureAfter:    -1,
		FailureCooldown: -time.Second,
	})

	require.Equal(t, DeliveryGateway, delivery.Mode)
	require.Equal(t, "slack", delivery.Channel)
	require.Equal(t, "C1", delivery.Target)
	require.Equal(t, "123", delivery.ThreadID)
	require.Equal(t, "https://example.com/hook", delivery.WebhookURL)
	require.Equal(t, "ops", delivery.FailureTarget)
	require.Zero(t, delivery.FailureAfter)
	require.Zero(t, delivery.FailureCooldown)

	require.Equal(t, DeliveryNone, normalizeDelivery(Delivery{}).Mode)
}

func TestCheckFailureNoticeDue(t *testing.T) {
	now := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	job := Job{State: JobState{ConsecutiveErrors: 1}}

	require.False(t, checkFailureNoticeDue(job, Delivery{}, now))
	require.False(t, checkFailureNoticeDue(job, Delivery{FailureAfter: 3}, now))
	require.True(t, checkFailureNoticeDue(job, Delivery{FailureAfter: 2}, now))

	job.State.LastFailureNoticeAt = now.Add(-30 * time.Minute)
	require.False(t, checkFailureNoticeDue(job, Delivery{FailureAfter: 2, FailureCooldown: time.Hour}, now))
	require.True(t, checkFailureNoticeDue(job, Delivery{FailureAfter: 2, FailureCooldown: time.Hour}, now.Add(31*time.Minute)))
}

func TestDeliverWebhookFailures(t *testing.T) {
	err := deliverWebhook(context.Background(), nil, " ", DeliveryRequest{})
	require.EqualError(t, err, "automation webhook URL is required")

	err = deliverWebhook(context.Background(), nil, "://bad-url", DeliveryRequest{})
	require.ErrorContains(t, err, "missing protocol scheme")

	expected := errors.New("network failed")
	err = deliverWebhook(
		context.Background(),
		deliveryHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return nil, expected
		}),
		"https://example.com/hook",
		DeliveryRequest{},
	)
	require.ErrorIs(t, err, expected)

	err = deliverWebhook(
		context.Background(),
		deliveryHTTPClientFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
		"https://example.com/hook",
		DeliveryRequest{},
	)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad hook", http.StatusBadGateway)
	}))
	defer server.Close()

	err = deliverWebhook(context.Background(), server.Client(), server.URL, DeliveryRequest{})
	require.EqualError(t, err, "automation webhook delivery failed: 502 Bad Gateway: bad hook")

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	err = deliverWebhook(context.Background(), server.Client(), server.URL, DeliveryRequest{})
	require.EqualError(t, err, "automation webhook delivery failed: 502 Bad Gateway")
}

func TestDeliverWebhook_UsesCamelCaseJSON(t *testing.T) {
	var body []byte
	client := deliveryHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		var err error
		body, err = io.ReadAll(req.Body)
		require.NoError(t, err)

		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	err := deliverWebhook(
		context.Background(),
		client,
		"https://example.com/hook",
		DeliveryRequest{
			JobID:     "job",
			RunID:     "run",
			Status:    RunStatusOK,
			Output:    "done",
			SessionID: "session",
			Target: DeliveryTarget{
				Mode:     DeliveryWebhook,
				ThreadID: "thread",
			},
		},
	)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"jobId": "job",
		"runId": "run",
		"status": "ok",
		"output": "done",
		"error": "",
		"sessionId": "session",
		"target": {
			"mode": "webhook",
			"channel": "",
			"target": "",
			"threadId": "thread"
		}
	}`, string(body))
}
