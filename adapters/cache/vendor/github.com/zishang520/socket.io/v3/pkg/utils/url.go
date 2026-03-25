package utils

import (
	"fmt"
	"net"
	"net/url"
)

type ParsedUrl struct {
	*url.URL

	Hostname string
	Port     string
	Id       string
}

func Url(uri string, path string) (parsedUrl *ParsedUrl, err error) {
	url, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	parsedUrl = &ParsedUrl{URL: url}
	parsedUrl.Hostname = parsedUrl.URL.Hostname()
	parsedUrl.Port = parsedUrl.URL.Port()

	if parsedUrl.Port == "" {
		switch parsedUrl.Scheme {
		case "http", "ws":
			parsedUrl.Port = "80"
		case "https", "wss":
			parsedUrl.Port = "443"
		}
	}

	if parsedUrl.Path == "" {
		parsedUrl.Path = "/"
	}

	parsedUrl.Id = fmt.Sprintf("%s://%s%s", parsedUrl.Scheme, net.JoinHostPort(parsedUrl.Hostname, parsedUrl.Port), path)

	return parsedUrl, nil
}
