package episodic

import (
	"testing"

	"github.com/stretchr/testify/require"

	storage "github.com/wandxy/hand/internal/state/core"
)

func TestCandidateRejectionReason_RejectsExecutionDetail(t *testing.T) {
	reason := candidateRejectionReason(storage.MemoryItem{
		Metadata: map[string]string{
			"memory_granularity": " execution_detail ",
		},
	})

	require.Equal(t, "execution_detail", reason)
}
