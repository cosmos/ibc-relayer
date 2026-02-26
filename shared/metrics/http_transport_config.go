package metrics

import (
	"net/http"
	"strings"
)

type TransportConfig struct {
	// HTTPMethodExtractor returns the method that this HTTP request is for.
	// This is not the actual HTTP method (GET, POST, PUT, etc), but rather an
	// alias for the function that is being called, like /get/user or
	// /create/organization. The default is to use the requests path.
	HTTPMethodExtractor func(request *http.Request) string
}

var DefaultTransportConfig = TransportConfig{
	HTTPMethodExtractor: DefaultHTTPMethodExtractor,
}

func DefaultHTTPMethodExtractor(request *http.Request) string {
	return request.URL.Path
}

func AllButLastPathHTTPMethodExtractor(request *http.Request) string {
	fullPath := request.URL.Path
	lastSlashIndex := strings.LastIndex(fullPath, "/")
	if lastSlashIndex == -1 {
		return fullPath
	}
	return fullPath[:lastSlashIndex]
}
