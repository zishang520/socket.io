package types

import (
	"net/http"
	"regexp"
	"strings"
)

type (
	Cors struct {
		// Supported types: string, []string, []any, *regexp.Regexp, bool, func(string) bool
		Origin any
		// Supported types: string, []string
		Methods any
		// Supported types: nil, string, []string
		AllowedHeaders any
		// Supported types: nil, string, []string
		Headers any
		// Supported types: string, []string
		ExposedHeaders       any
		MaxAge               string
		Credentials          bool
		PreflightContinue    bool
		OptionsSuccessStatus int
	}

	Kv struct {
		Key   string
		Value string
	}
)

func (c *Cors) IsOriginAllowed(origin string, allowedOrigin any) bool {
	switch v := allowedOrigin.(type) {
	case []any:
		for _, value := range v {
			if c.IsOriginAllowed(origin, value) {
				return true
			}
		}
	case []string:
		for _, value := range v {
			if strings.EqualFold(origin, value) {
				return true
			}
		}
	case func(string) bool:
		return v(origin)
	case string:
		if v == "*" {
			return true
		}
		return strings.EqualFold(origin, v)
	case *regexp.Regexp:
		return v.MatchString(origin)
	case bool:
		return v
	}
	return false
}

// setCorsHeaderList accepts the two public header-list forms and omits empty lists.
func setCorsHeaderList(headers *ParameterBag, key string, value any) {
	switch values := value.(type) {
	case string:
		if len(values) > 0 {
			headers.Set(key, values)
		}
	case []string:
		if len(values) > 0 {
			headers.Set(key, strings.Join(values, ","))
		}
	}
}

func parseVary(vary string) map[string]struct{} {
	end := 0
	start := 0
	list := make(map[string]struct{})

	// gather tokens
	for i, l := 0, len(vary); i < l; i++ {
		switch vary[i] {
		case ' ': /*   */
			if start == end {
				end = i + 1
				start = end
			}
		case ',': /* , */
			list[vary[start:end]] = struct{}{}
			end = i + 1
			start = end
		default:
			end = i + 1
		}
	}

	if end := vary[start:end]; len(end) > 0 {
		// final token
		list[end] = struct{}{}
	}

	return list
}

func CorsMiddleware(options *Cors, ctx *HttpContext, next func(error)) {
	method := ctx.Method()
	requestOrigin := ctx.Headers().Peek("Origin")
	origin, fixed := options.Origin.(string)
	vary := make([]string, 0, 2)
	var allowedOrigin string
	if requestOrigin != "" && fixed && origin == "*" && !options.Credentials {
		allowedOrigin = "*"
	} else {
		vary = append(vary, "Origin")
		if requestOrigin != "" && options.IsOriginAllowed(requestOrigin, options.Origin) {
			allowedOrigin = origin
			if !fixed || origin == "*" {
				allowedOrigin = requestOrigin
			}
		}
	}

	// Resolve the origin callback before accessing or changing response headers.
	headers := ctx.ResponseHeaders()
	if allowedOrigin != "" {
		headers.Set("Access-Control-Allow-Origin", allowedOrigin)
	}
	if options.Credentials {
		headers.Set("Access-Control-Allow-Credentials", "true")
	}
	if method == http.MethodOptions {
		switch methods := options.Methods.(type) {
		case string:
			headers.Set("Access-Control-Allow-Methods", methods)
		case []string:
			headers.Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
		}
		allowedHeaders := options.AllowedHeaders
		if allowedHeaders == nil {
			allowedHeaders = options.Headers
		}
		if allowedHeaders == nil {
			if requested := ctx.Headers().Peek("Access-Control-Request-Headers"); requested != "" {
				headers.Set("Access-Control-Allow-Headers", requested)
				vary = append(vary, "Access-Control-Request-Headers")
			}
		} else {
			setCorsHeaderList(headers, "Access-Control-Allow-Headers", allowedHeaders)
		}
		if options.MaxAge != "" {
			headers.Set("Access-Control-Max-Age", options.MaxAge)
		}
	} else {
		setCorsHeaderList(headers, "Access-Control-Expose-Headers", options.ExposedHeaders)
	}

	if current := headers.Peek("Vary"); current == "*" {
		headers.Set("Vary", "*")
	} else if len(vary) > 0 {
		values := parseVary(current)
		for _, value := range vary {
			values[value] = struct{}{}
		}
		keys := make([]string, 0, len(values))
		for value := range values {
			keys = append(keys, value)
		}
		headers.Set("Vary", strings.Join(keys, ", "))
	}

	if method == http.MethodOptions && !options.PreflightContinue {
		headers.Set("Content-Length", "0")
		_ = ctx.SetStatusCode(options.OptionsSuccessStatus)
		_, _ = ctx.Write(nil)
		return
	}
	next(nil)
}

var defaultCors = &Cors{
	Origin:               `*`,
	Methods:              `GET,HEAD,PUT,PATCH,POST,DELETE`,
	PreflightContinue:    false,
	OptionsSuccessStatus: 204,
}

func MiddlewareWrapper(options *Cors) func(*HttpContext, func(error)) {
	if options == nil {
		options = defaultCors
	} else {
		if options.Origin == nil {
			options.Origin = "*"
		}

		if options.Methods == nil {
			options.Methods = `GET,HEAD,PUT,PATCH,POST,DELETE`
		}

		if options.OptionsSuccessStatus == 0 {
			options.OptionsSuccessStatus = http.StatusNoContent
		}
	}

	return func(ctx *HttpContext, next func(error)) {
		CorsMiddleware(options, ctx, next)
	}
}
