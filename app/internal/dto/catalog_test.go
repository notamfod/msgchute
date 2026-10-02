package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageCatalogsMatchValidation(t *testing.T) {
	require.Equal(t, []MessageStatus{MessageStatusRunning, MessageStatusSucceeded, MessageStatusFailed, MessageStatusDeclined}, MessageStatuses())
	require.Equal(t, []string{"order", "promotion", "news"}, MessageTags())
	for _, tag := range MessageTags() {
		require.True(t, ValidMessageTag(tag))
	}
	require.False(t, ValidMessageTag("unknown"))
}
