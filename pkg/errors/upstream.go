package errors

type UpstreamError struct {
	Code   int
	Msg    string
	reason ErrorReason
}

func NewUpstreamError(Code int, Msg string, reason ErrorReason) *UpstreamError {
	return &UpstreamError{Code: Code, Msg: Msg, reason: reason}
}

func (ue *UpstreamError) Error() string {
	return ue.Msg
}

func (ue *UpstreamError) Reason() ErrorReason {
	return ue.reason
}
