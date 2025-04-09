package socket

type ExtendedError struct {
	Message string `json:"message" msgpack:"message"`
	Data    any    `json:"data" msgpack:"data"`
}

func NewExtendedError(message string, data any) *ExtendedError {
	return &ExtendedError{Message: message, Data: data}
}

func (e *ExtendedError) Err() error {
	return e
}

func (e *ExtendedError) Error() string {
	return e.Message
}
