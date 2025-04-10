package types

type (
	CodeMessage struct {
		Code    int    `json:"code" msgpack:"code"`
		Message string `json:"message,omitempty" msgpack:"message,omitempty"`
	}

	ErrorMessage struct {
		*CodeMessage

		Req     *HttpContext   `json:"req,omitempty" msgpack:"req,omitempty"`
		Context map[string]any `json:"context,omitempty" msgpack:"context,omitempty"`
	}
)
