package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func BenchmarkTranscriptRender(b *testing.B) {
	for _, count := range []int{10, 40, 120, 500} {
		b.Run(fmt.Sprintf("full_%d_cells", count), func(b *testing.B) {
			cells := benchmarkTranscriptCells(count)
			b.ResetTimer()
			for range b.N {
				_ = renderTranscriptCellsWithFrame(cells, 120, 0)
			}
		})
	}

	b.Run("cached_500_cells_changed_tail", func(b *testing.B) {
		cells := benchmarkTranscriptCells(500)
		cache := newTranscriptRenderCache(defaultTranscriptRenderCacheCapacity)
		_ = renderTranscriptCellsWithFrameAndCache(cells, 120, 0, cache)
		b.ResetTimer()
		for index := range b.N {
			cells[len(cells)-1] = assistantTranscriptCell{text: fmt.Sprintf("tail %d", index)}
			_ = renderTranscriptCellsWithFrameAndCache(cells, 120, index, cache)
		}
	})
}

func BenchmarkResponseEventBatch64(b *testing.B) {
	messages := make([]tea.Msg, 64)
	for index := range messages {
		messages[index] = assistantTextDeltaMsg{Text: "token "}
	}

	runModel := newModel()
	runModel.messages = benchmarkTranscriptCells(500)
	runModel.responding = true
	runModel.responseID = 1
	runModel.setTranscriptContent()
	b.ResetTimer()
	for index := range b.N {
		b.StopTimer()
		runModel.stream.Reset()
		runModel.applyAction(setLiveTranscriptCellAction{})
		runModel.streamingRenderAt = time.Time{}
		messages[len(messages)-1] = assistantTextDeltaMsg{Text: fmt.Sprintf("tail %d", index)}
		b.StartTimer()
		updated, _ := runModel.handleResponseEventBatch(responseEventBatchMsg{
			ResponseID: 1,
			Messages:   messages,
		})
		runModel = updated.(model)
	}
}

func benchmarkTranscriptCells(count int) []transcriptCell {
	cells := make([]transcriptCell, count)
	for index := range cells {
		cells[index] = assistantTranscriptCell{
			text: fmt.Sprintf("## Message %d\n\n%s", index, strings.Repeat("rendered markdown content ", 3)),
		}
	}
	return cells
}
