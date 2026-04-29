package playground

import "errors"

var (
	ErrAgentNotFound     = errors.New("agent not found")
	ErrMaxAgentsReached  = errors.New("maximum agents per visitor reached")
	ErrAccessDenied       = errors.New("access denied")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrMaxConversationsReached = errors.New("maximum conversations per visitor reached")
)