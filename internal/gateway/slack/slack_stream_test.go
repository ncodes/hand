package slack

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgslack "github.com/wandxy/hand/pkg/gateway/slack"
)

func TestSender_StreamTurnUsesNativeSlackStream(t *testing.T) {
	api := &fakeSlackAPI{}
	sender := NewSender(api)
	target := slackTestTarget()

	err := sender.StreamTurn(context.Background(), target, func(onDelta func(string)) (string, error) {
		onDelta("hello ")
		onDelta("world")
		return "final", nil
	})

	require.NoError(t, err)
	require.Equal(t, []slackAPICall{
		{method: "startStream", target: target},
		{method: "appendStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}, text: "hello "},
		{method: "appendStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}, text: "world"},
		{method: "stopStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}},
	}, api.allCalls())
}

func TestSender_StreamTurnDoesNotBlockRunOnAppendLatency(t *testing.T) {
	api := &fakeSlackAPI{appendDelay: 100 * time.Millisecond}
	sender := NewSender(api)
	var runElapsed time.Duration

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		start := time.Now()
		onDelta("hello ")
		onDelta("world")
		runElapsed = time.Since(start)
		return "final", nil
	})

	require.NoError(t, err)
	require.Less(t, runElapsed, 50*time.Millisecond)
	require.Equal(t, []string{"startStream", "appendStream", "appendStream", "stopStream"}, api.callMethods())
}

func TestSender_StreamTurnFallsBackWhenStartFails(t *testing.T) {
	api := &fakeSlackAPI{startErr: errSlackTest}
	sender := NewSender(api)
	target := slackTestTarget()

	err := sender.StreamTurn(context.Background(), target, func(onDelta func(string)) (string, error) {
		onDelta("ignored")
		return "final", nil
	})

	require.NoError(t, err)
	require.Equal(t, []slackAPICall{
		{method: "startStream", target: target},
		{method: "postMessage", target: target, text: "final"},
	}, api.allCalls())
}

func TestSender_StreamTurnReturnsRunErrorWhenStartFails(t *testing.T) {
	api := &fakeSlackAPI{startErr: errSlackTest}
	sender := NewSender(api)

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		return "", errSlackTest
	})

	require.ErrorIs(t, err, errSlackTest)
	require.Len(t, api.allCalls(), 1)
}

func TestSender_StreamTurnChunksLargeDeltasWithoutTrimming(t *testing.T) {
	api := &fakeSlackAPI{}
	sender := NewSender(api)
	delta := strings.Repeat("a", pkgslack.MarkdownTextLimit) + " b"

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		onDelta(delta)
		return "final", nil
	})

	require.NoError(t, err)
	calls := api.allCalls()
	require.Len(t, calls, 4)
	require.Equal(t, strings.Repeat("a", pkgslack.MarkdownTextLimit), calls[1].text)
	require.Equal(t, " b", calls[2].text)
}

func TestSender_StreamTurnFallsBackWhenAppendFailsBeforeVisibleOutput(t *testing.T) {
	api := &fakeSlackAPI{appendErr: errSlackTest}
	sender := NewSender(api)
	target := slackTestTarget()

	err := sender.StreamTurn(context.Background(), target, func(onDelta func(string)) (string, error) {
		onDelta("first")
		onDelta("ignored")
		return "final", nil
	})

	require.NoError(t, err)
	require.Equal(t, []slackAPICall{
		{method: "startStream", target: target},
		{method: "appendStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}, text: "first"},
		{method: "postMessage", target: target, text: "final"},
	}, api.allCalls())
}

func TestSender_StreamTurnDoesNotFallbackWhenAppendFailsAfterVisibleOutput(t *testing.T) {
	api := &fakeSlackAPI{appendErrAfter: 1}
	sender := NewSender(api)
	delta := strings.Repeat("a", pkgslack.MarkdownTextLimit) + " b"

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		onDelta(delta)
		return "final", nil
	})

	require.NoError(t, err)
	require.Equal(t, []string{"startStream", "appendStream", "appendStream", "stopStream"}, api.callMethods())
}

func TestSender_StreamTurnReturnsStopErrorAfterVisibleOutput(t *testing.T) {
	api := &fakeSlackAPI{stopErr: errSlackTest}
	sender := NewSender(api)

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		onDelta("visible")
		return "final", nil
	})

	require.ErrorIs(t, err, errSlackTest)
}

func TestSender_StreamTurnFallsBackWhenStopFailsBeforeVisibleOutput(t *testing.T) {
	api := &fakeSlackAPI{stopErr: errSlackTest}
	sender := NewSender(api)

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		onDelta(" ")
		return "final", nil
	})

	require.NoError(t, err)
	require.Equal(t, "postMessage", api.allCalls()[2].method)
}

func TestSender_StreamTurnReturnsRunError(t *testing.T) {
	api := &fakeSlackAPI{}
	sender := NewSender(api)

	err := sender.StreamTurn(context.Background(), slackTestTarget(), func(onDelta func(string)) (string, error) {
		onDelta("visible")
		return "", errSlackTest
	})

	require.ErrorIs(t, err, errSlackTest)
}

func TestSender_SendFinalChunksLongText(t *testing.T) {
	api := &fakeSlackAPI{}
	sender := NewSender(api)
	target := slackTestTarget()
	text := strings.Repeat("a", pkgslack.MarkdownTextLimit+1)

	err := sender.SendFinal(context.Background(), target, text)

	require.NoError(t, err)
	calls := api.allCalls()
	require.Len(t, calls, 2)
	require.Equal(t, pkgslack.MarkdownTextLimit, len(calls[0].text))
	require.Equal(t, 1, len(calls[1].text))
}

func TestSender_SendFinalReturnsPostError(t *testing.T) {
	api := &fakeSlackAPI{postErr: errSlackTest}

	err := NewSender(api).SendFinal(context.Background(), slackTestTarget(), "hello")

	require.ErrorIs(t, err, errSlackTest)
}

func TestSender_SendFinalSkipsEmptyText(t *testing.T) {
	api := &fakeSlackAPI{}

	err := NewSender(api).SendFinal(context.Background(), slackTestTarget(), " ")

	require.NoError(t, err)
	require.Empty(t, api.allCalls())
}

func TestSender_RequiresAPI(t *testing.T) {
	err := (*Sender)(nil).SendFinal(context.Background(), slackTestTarget(), "hello")
	require.EqualError(t, err, "slack sender is required")

	err = NewSender(nil).StreamTurn(context.Background(), slackTestTarget(), func(func(string)) (string, error) {
		return "", nil
	})
	require.EqualError(t, err, "slack sender is required")
}

func TestSlackStreamAppender_DefaultsInvalidInterval(t *testing.T) {
	appender := newSlackStreamAppender(context.Background(), &fakeSlackAPI{}, pkgslack.Stream{}, 0)

	require.Equal(t, defaultSlackStreamFlushInterval, appender.interval)
}

func TestSlackStreamAppender_FlushesOnTicker(t *testing.T) {
	api := &fakeSlackAPI{}
	appender := newSlackStreamAppender(
		context.Background(),
		api,
		pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"},
		time.Millisecond,
	)
	appender.Start()
	appender.Append("hello")

	require.Eventually(t, func() bool {
		return len(api.allCalls()) == 1
	}, time.Second, 10*time.Millisecond)

	err, visible := appender.Stop()
	require.NoError(t, err)
	require.True(t, visible)
	require.Equal(t, []slackAPICall{
		{method: "appendStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}, text: "hello"},
	}, api.allCalls())
}

func TestSlackStreamAppender_FlushesOnContextCancellation(t *testing.T) {
	api := &fakeSlackAPI{}
	ctx, cancel := context.WithCancel(context.Background())
	appender := newSlackStreamAppender(
		ctx,
		api,
		pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"},
		time.Hour,
	)
	appender.Start()
	appender.Append("hello")
	cancel()

	require.Eventually(t, func() bool {
		return len(api.allCalls()) == 1
	}, time.Second, 10*time.Millisecond)

	err, visible := appender.Stop()
	require.NoError(t, err)
	require.True(t, visible)
}

func TestSlackStreamAppender_SkipsBlankAndAfterError(t *testing.T) {
	api := &fakeSlackAPI{appendErr: errSlackTest}
	appender := newSlackStreamAppender(
		context.Background(),
		api,
		pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"},
		time.Hour,
	)
	appender.Start()
	appender.Append(" ")
	appender.Append("first")
	appender.Append("ignored")

	err, visible := appender.Stop()

	require.ErrorIs(t, err, errSlackTest)
	require.False(t, visible)
	appender.Append("ignored")
	require.Equal(t, []slackAPICall{
		{method: "appendStream", stream: pkgslack.Stream{ChannelID: "C1", TS: "stream-ts"}, text: "first"},
	}, api.allCalls())
}

func TestGetSlackStreamChunksSkipsBlankText(t *testing.T) {
	require.Nil(t, getSlackStreamChunks("   "))
}

func slackTestTarget() pkgslack.Target {
	return pkgslack.Target{
		TeamID:          "T1",
		ChannelID:       "C1",
		ThreadTS:        "100.1",
		UserID:          "U1",
		ChannelType:     "channel",
		RecipientUserID: "U1",
		RecipientTeamID: "T1",
	}
}
