package statusdata

// JobReview is bound by an opaque, one-use token inside the operations backend.
type JobReview struct {
	ID, Token, Destination, ConfigFingerprint, Reason string
	Sources                                           int
	Allowed, Notifications                            bool
}
type JobEvent struct {
	Phase, State, Message string
	Done                  bool
}
