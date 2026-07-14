package permissions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApprovalService_NotifyDoesNotBlockOnSlowWaiter(t *testing.T) {
	waiter := make(chan ApprovalRequest, 1)
	waiter <- ApprovalRequest{ID: "existing"}
	service := &ApprovalService{waiters: map[string][]chan ApprovalRequest{"approval": {waiter}}}
	done := make(chan struct{})
	go func() {
		service.notify(ApprovalRequest{ID: "approval"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "notification blocked on a slow waiter")
	}
	require.Equal(t, "existing", (<-waiter).ID)
}
