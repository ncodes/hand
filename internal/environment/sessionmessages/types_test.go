package sessionmessages

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionMessagesRequest_ValidateAcceptsSingleSelectorModes(t *testing.T) {
	offsetStart := 0
	offsetEnd := 2

	require.NoError(t, SessionMessagesRequest{
		MessageIDs: []uint{1, 2},
	}.Validate())
	require.NoError(t, SessionMessagesRequest{
		AnchorMessageID: 5,
		Before:          1,
		After:           2,
	}.Validate())
	require.NoError(t, SessionMessagesRequest{
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
	}.Validate())
}

func TestSessionMessagesRequest_ValidateRejectsAmbiguousOrInvalidSelectors(t *testing.T) {
	offsetStart := 0
	offsetEnd := 2

	require.EqualError(t, SessionMessagesRequest{}.Validate(), "exactly one session message selector must be provided")
	require.EqualError(t, SessionMessagesRequest{
		MessageIDs:      []uint{1},
		AnchorMessageID: 2,
	}.Validate(), "exactly one session message selector must be provided")
	require.EqualError(t, SessionMessagesRequest{
		OffsetStart: &offsetStart,
	}.Validate(), "offset_start and offset_end are required together")
	require.EqualError(t, SessionMessagesRequest{
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetStart,
	}.Validate(), "offset_end must be greater than offset_start")
	require.EqualError(t, SessionMessagesRequest{
		AnchorMessageID: 1,
		Before:          -1,
	}.Validate(), "before and after must be greater than or equal to zero")
	require.EqualError(t, SessionMessagesRequest{
		MessageIDs: []uint{0},
	}.Validate(), "message_ids must contain only positive ids")
	require.EqualError(t, SessionMessagesRequest{
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
		Before:      1,
	}.Validate(), "before and after are only supported with anchor_message_id")
}

func TestSessionMessagesRequest_ValidateRejectsNegativeMaxChars(t *testing.T) {
	require.EqualError(t, SessionMessagesRequest{
		MessageIDs: []uint{1},
		MaxChars:   -1,
	}.Validate(), "max_chars must be greater than or equal to zero")
}

func TestSessionMessagesRequest_ValidateRejectsOffsetsWithMessageIDs(t *testing.T) {
	offsetStart := 0
	offsetEnd := 2

	require.EqualError(t, SessionMessagesRequest{
		MessageIDs:  []uint{1},
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
	}.Validate(), "exactly one session message selector must be provided")
}

func TestSessionMessagesRequest_ValidateRejectsOffsetsWithAnchorSelector(t *testing.T) {
	offsetStart := 0
	offsetEnd := 2

	require.EqualError(t, SessionMessagesRequest{
		AnchorMessageID: 1,
		OffsetStart:     &offsetStart,
		OffsetEnd:       &offsetEnd,
	}.Validate(), "exactly one session message selector must be provided")
}

func TestSessionMessagesRequest_ValidateRejectsBeforeAfterWithMessageIDs(t *testing.T) {
	require.EqualError(t, SessionMessagesRequest{
		MessageIDs: []uint{1},
		Before:     1,
	}.Validate(), "before and after are only supported with anchor_message_id")
}

func TestSessionMessagesRequest_ValidateRejectsNegativeOffsetStart(t *testing.T) {
	offsetStart := -1
	offsetEnd := 2

	require.EqualError(t, SessionMessagesRequest{
		OffsetStart: &offsetStart,
		OffsetEnd:   &offsetEnd,
	}.Validate(), "offset_start must be greater than or equal to zero")
}
