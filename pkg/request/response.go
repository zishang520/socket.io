package request

import "resty.dev/v3"

type Response struct {
	*resty.Response
}

func (r *Response) Ok() bool {
	return r.StatusCode() >= 200 && r.StatusCode() <= 299
}
