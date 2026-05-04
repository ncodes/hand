package episodic

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

func TestNewLLMExtractor_ValidatesOptions(t *testing.T) {
	_, err := NewLLMExtractor(LLMExtractorOptions{})
	require.EqualError(t, err, "memory episode extractor model client is required")

	_, err = NewLLMExtractor(LLMExtractorOptions{Client: &llmExtractorClientStub{}})
	require.EqualError(t, err, "memory episode extractor model is required")

	extractor, err := NewLLMExtractor(LLMExtractorOptions{
		Client: &llmExtractorClientStub{},
		Model:  "test-model",
	})
	require.NoError(t, err)
	require.Equal(t, defaultLLMExtractorMaxOutputTokens, extractor.options.MaxOutputTokens)
}

func TestLLMExtractor_ExtractCandidatesUsesStructuredRequestAndParsesResponse(t *testing.T) {
	client := &llmExtractorClientStub{
		response: &models.Response{OutputText: `{
			"candidates": [{
				"kind": "decision",
				"title": " Decision ",
				"text": " Use LLM extraction only. ",
				"confidence": 0.91,
				"metadata": {"decision": "llm_only"}
			}],
			"rejections": [{"kind": "window", "reason": "low_signal_line"}]
		}`},
	}
	extractor, err := NewLLMExtractor(LLMExtractorOptions{
		Client:          client,
		Model:           "test-model",
		APIMode:         models.APIModeResponses,
		MaxOutputTokens: 42,
		DebugRequests:   true,
	})
	require.NoError(t, err)

	result, err := extractor.ExtractCandidates(context.Background(), CandidateRequest{
		SessionID: "session",
		Start:     1,
		End:       2,
		Messages:  []string{"user: use LLM extraction only"},
		TraceEvents: []taskTraceEvidence{{
			Ref:     "trace:2",
			Type:    "tool.invocation.completed",
			Payload: `{"name":"run_command","exit_code":0}`,
		}},
		MaxChars: 500,
	})

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, episodeKindDecision, result.Candidates[0].Kind)
	require.Equal(t, "Decision", result.Candidates[0].Title)
	require.Equal(t, "Use LLM extraction only.", result.Candidates[0].Text)
	require.Equal(t, "llm_only", result.Candidates[0].Metadata["decision"])
	require.Equal(t, []candidateRejection{{Kind: "window", Reason: "low_signal_line"}}, result.Rejections)
	require.Len(t, client.requests, 1)
	require.Equal(t, "test-model", client.requests[0].Model)
	require.Equal(t, models.APIModeResponses, client.requests[0].APIMode)
	require.Equal(t, int64(42), client.requests[0].MaxOutputTokens)
	require.True(t, client.requests[0].DebugRequests)
	require.NotNil(t, client.requests[0].StructuredOutput)
	require.Contains(t, client.requests[0].Instructions, "Do not store raw transcript windows")
	require.Len(t, client.requests[0].Messages, 1)
	require.Equal(t, handmsg.RoleUser, client.requests[0].Messages[0].Role)
	require.Contains(t, client.requests[0].Messages[0].Content, `"session_id":"session"`)
	require.Contains(t, client.requests[0].Messages[0].Content, `"trace_events"`)
	require.Contains(t, client.requests[0].Messages[0].Content, `"ref":"trace:2"`)
}

func TestLLMExtractor_ExtractCandidatesParsesFencedJSONResponse(t *testing.T) {
	client := &llmExtractorClientStub{
		response: &models.Response{OutputText: "```json\n" + `{
			"candidates": [{
				"kind": "outcome",
				"title": "Outcome",
				"text": "Implemented fenced JSON parsing.",
				"confidence": 0.82,
				"metadata": {}
			}],
			"rejections": []
		}` + "\n```"},
	}
	extractor, err := NewLLMExtractor(LLMExtractorOptions{
		Client: client,
		Model:  "test-model",
	})
	require.NoError(t, err)

	result, err := extractor.ExtractCandidates(context.Background(), CandidateRequest{})

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, episodeKindOutcome, result.Candidates[0].Kind)
	require.Equal(t, "Implemented fenced JSON parsing.", result.Candidates[0].Text)
}

func TestLLMExtractor_ExtractCandidatesReturnsClientAndParseErrors(t *testing.T) {
	_, err := (*LLMExtractor)(nil).ExtractCandidates(context.Background(), CandidateRequest{})
	require.EqualError(t, err, "memory episode extractor model client is required")

	extractor, err := NewLLMExtractor(LLMExtractorOptions{
		Client: &llmExtractorClientStub{err: errors.New("model failed")},
		Model:  "test-model",
	})
	require.NoError(t, err)

	_, err = extractor.ExtractCandidates(context.Background(), CandidateRequest{})
	require.EqualError(t, err, "model failed")

	extractor, err = NewLLMExtractor(LLMExtractorOptions{
		Client: &llmExtractorClientStub{response: nil},
		Model:  "test-model",
	})
	require.NoError(t, err)

	_, err = extractor.ExtractCandidates(context.Background(), CandidateRequest{})
	require.EqualError(t, err, "memory episode extractor response is required")

	extractor, err = NewLLMExtractor(LLMExtractorOptions{
		Client: &llmExtractorClientStub{response: &models.Response{OutputText: "{"}},
		Model:  "test-model",
	})
	require.NoError(t, err)

	_, err = extractor.ExtractCandidates(context.Background(), CandidateRequest{})
	require.Error(t, err)
}

func TestLLMExtractorStructuredOutputUsesLowercaseRejectionFields(t *testing.T) {
	output := llmExtractorStructuredOutput()
	require.NotNil(t, output)
	require.True(t, output.Strict)

	properties := output.Schema["properties"].(map[string]any)
	candidates := properties["candidates"].(map[string]any)
	candidateItems := candidates["items"].(map[string]any)
	candidateProperties := candidateItems["properties"].(map[string]any)
	kinds := candidateProperties["kind"].(map[string]any)
	rejections := properties["rejections"].(map[string]any)
	items := rejections["items"].(map[string]any)
	rejectionProperties := items["properties"].(map[string]any)

	require.ElementsMatch(t, episodeCandidateKinds(), kinds["enum"])
	require.Contains(t, rejectionProperties, "kind")
	require.Contains(t, rejectionProperties, "reason")
	require.NotContains(t, rejectionProperties, "Kind")
	require.NotContains(t, rejectionProperties, "Reason")
}

type llmExtractorClientStub struct {
	requests []models.Request
	response *models.Response
	err      error
}

func (s *llmExtractorClientStub) Complete(_ context.Context, req models.Request) (*models.Response, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.response, nil
}

func (s *llmExtractorClientStub) CompleteStream(
	context.Context,
	models.Request,
	func(models.StreamDelta),
) (*models.Response, error) {
	return nil, errors.New("streaming is not supported")
}
