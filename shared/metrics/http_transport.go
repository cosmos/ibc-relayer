package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

type MetricsTrasportMiddleware struct {
	internal http.RoundTripper
	config   *TransportConfig
}

func NewTransportMiddleware(internal http.RoundTripper, config *TransportConfig) *MetricsTrasportMiddleware {
	if config == nil {
		config = &DefaultTransportConfig
	}
	return &MetricsTrasportMiddleware{
		internal: internal,
		config:   config,
	}
}

// RoundTrip records metrics for json rpc and rest http requests.
func (t *MetricsTrasportMiddleware) RoundTrip(request *http.Request) (*http.Response, error) {
	if isJSONRPCRequest(request) {
		return t.JSONRPCRoundTrip(request)
	}
	return t.HTTPRoundTrip(request)
}

// JSONRPC records metrics for an http request that is a json rpc request. This
// will record the requests latency, and if the request was successful or not.
// For JSON RPC requests, the requests method is the method of the rpc call
// specified in the body. The provider is the hostname of the requests url. And
// the code is the json rpc error code in the case of a successful request that
// received an error response in the body, "-1" if the request was
// unsuccessful, and the http status code as a string in the case of a
// successful request with no error.
// and "-1" if unsuccessful.
func (t *MetricsTrasportMiddleware) JSONRPCRoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	jsonRPCMethod := getJSONRPCMethodFromRequest(request)
	provider := request.URL.Hostname()

	start := time.Now()
	defer func() {
		FromContext(ctx).ExternalRequestLatency(ctx, jsonRPCMethod, provider, time.Since(start))
	}()

	response, err := t.internal.RoundTrip(request)
	if err != nil {
		if isExternalError(err) {
			FromContext(ctx).AddFailedExternalRequest(ctx, jsonRPCMethod, "-1", provider)
		} else {
			FromContext(ctx).AddSuccessfulExternalRequest(ctx, jsonRPCMethod, "-1", provider)
		}
		return nil, err
	}

	var code string
	if response != nil {
		code = getJSONRPCCodeFromResponse(response)
	}

	FromContext(ctx).AddSuccessfulExternalRequest(ctx, jsonRPCMethod, code, provider)
	return response, nil
}

// HTTPRoundTrip records metrics for an http request that is not a json rpc
// request. This will record the requests latency, and if the request was
// successful or not. For HTTP requests, the requests method is the request
// path, the provider is the hostname of the requests url, and the code is the
// http status code as a string if the request is successful, and "-1" if
// unsuccessful.
func (t *MetricsTrasportMiddleware) HTTPRoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	method := t.config.HTTPMethodExtractor(request)
	provider := request.URL.Hostname()

	start := time.Now()
	defer func() {
		FromContext(ctx).ExternalRequestLatency(ctx, method, provider, time.Since(start))
	}()

	response, err := t.internal.RoundTrip(request)
	if err != nil {
		if isExternalError(err) {
			FromContext(ctx).AddFailedExternalRequest(ctx, method, "-1", provider)
		} else {
			FromContext(ctx).AddSuccessfulExternalRequest(ctx, method, "-1", provider)
		}
		return nil, err
	}

	var code string
	if response != nil {
		code = strconv.Itoa(response.StatusCode)
	}
	FromContext(ctx).AddSuccessfulExternalRequest(ctx, method, code, provider)
	return response, nil
}

func getJSONRPCMethodFromRequest(request *http.Request) string {
	if request.Body == nil {
		return ""
	}

	bytes, err := requestBodyBytes(request)
	if err != nil {
		return "error: failed to read request body"
	}

	var jsonRPCBody struct{ Method string }
	if err := json.Unmarshal(bytes, &jsonRPCBody); err != nil {
		return "error: failed to unmarshal request body"
	}

	return jsonRPCBody.Method
}

func getJSONRPCCodeFromResponse(response *http.Response) string {
	if response.Body == nil {
		return ""
	}

	bytes, err := responseBodyBytes(response)
	if err != nil {
		return "error: failed to read response body"
	}

	var jsonRPCResponse struct{ Error *struct{ Code int } }
	if err := json.Unmarshal(bytes, &jsonRPCResponse); err != nil {
		return "error: failed to unmarshal response body"
	}

	if jsonRPCResponse.Error == nil {
		// if no error, use the http status code
		return http.StatusText(response.StatusCode)
	}

	return strconv.Itoa(jsonRPCResponse.Error.Code)
}

func isJSONRPCRequest(request *http.Request) bool {
	if request.Body == nil {
		return false
	}

	bytes, err := requestBodyBytes(request)
	if err != nil {
		return false
	}

	// attempt to unmarshal the body into a JSON-RPC request
	type jsonRPCRequest struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	var req jsonRPCRequest
	if err := json.Unmarshal(bytes, &req); err != nil {
		return false
	}

	// check if the required fields are present
	return req.JSONRPC == "2.0" && req.Method != ""
}

func requestBodyBytes(request *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(request.Body, &buf)
	defer func() {
		request.Body = io.NopCloser(&buf)
	}()

	return io.ReadAll(tee)
}

func responseBodyBytes(response *http.Response) ([]byte, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(response.Body, &buf)
	defer func() {
		response.Body = io.NopCloser(&buf)
	}()

	return io.ReadAll(tee)
}

func isExternalError(err error) bool {
	return !errors.Is(err, context.Canceled)
}
