package utils

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	ErrEmptyURI          = errors.New("URI must not be empty")
	ErrUnsupportedScheme = errors.New("unsupported URI scheme")
)

type ParsedUrl struct {
	*url.URL

	Hostname string
	Port     string
	Id       string
}

func Url(uri string, path string) (*ParsedUrl, error) {
	if uri == "" {
		return nil, ErrEmptyURI
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var defaultPort string
	switch parsed.Scheme {
	case "http", "ws":
		defaultPort = "80"
	case "https", "wss":
		defaultPort = "443"
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedScheme, parsed.Scheme)
	}

	parsedUrl := &ParsedUrl{URL: parsed, Hostname: parsed.Hostname(), Port: parsed.Port()}

	if parsedUrl.Port == "" {
		parsedUrl.Port = defaultPort
	}

	if parsedUrl.Path == "" {
		parsedUrl.Path = "/"
	}

	parsedUrl.Id = parsedUrl.Scheme + "://" + net.JoinHostPort(parsedUrl.Hostname, parsedUrl.Port) + path

	return parsedUrl, nil
}
