package dto

import "slices"

var messageTags = []string{"order", "promotion", "news"}

func MessageStatuses() []MessageStatus {
	return []MessageStatus{MessageStatusRunning, MessageStatusSucceeded, MessageStatusFailed, MessageStatusDeclined}
}

func MessageTags() []string { return append([]string(nil), messageTags...) }

func ValidMessageTag(tag string) bool { return slices.Contains(messageTags, tag) }
