package putxt

import "fmt"

const (
	MessageRatelimited   = "RATELIMIT_REACHED"
	MessageSizeLimited   = "MAX_SIZE_EXCEEDED"
	MessageBinaryData    = "BINARY_DATA_REJECTED"
	MessageInternalError = "INTERNAL_ERROR"
)

type Logger interface {
	Printf(format string, v ...interface{})
}

type dummyLogger struct{}

func (dummyLogger) Printf(format string, v ...interface{}) {
	_, _ = fmt.Printf(format, v...)
}
