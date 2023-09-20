package lambdaproxyhttpadapter_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/jfallis/lambda-proxy-http-adapter"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
)

func TestGetHTTPHandler(t *testing.T) {
	type response struct {
		statusCode int
		body       string
	}
	tests := map[string]struct {
		handler http.Handler
		response
	}{
		"GetHTTPHandler with pointer response": {
			handler: adapter.GetHTTPHandler(
				func(r events.APIGatewayProxyRequest) (any, error) {
					handlerAssertions(t, &r)
					response := mockHandlerResponse()
					return &response, nil
				}, "/users/{userId}", map[string]string{"var1": "varValue1"},
			),
			response: response{
				statusCode: http.StatusOK,
				body:       "response_body",
			},
		},
		"GetHTTPHandler without pointer response": {
			handler: adapter.GetHTTPHandler(func(r events.APIGatewayProxyRequest) (any, error) {
				handlerAssertions(t, &r)
				return mockHandlerResponse(), nil
			}, "/users/{userId}", map[string]string{"var1": "varValue1"},
			),
			response: response{
				statusCode: http.StatusOK,
				body:       "response_body",
			},
		},
		"GetHTTPHandler with invalid response": {
			handler: adapter.GetHTTPHandler(func(r events.APIGatewayProxyRequest) (any, error) {
				handlerAssertions(t, &r)
				return "", nil
			}, "/users/{userId}", map[string]string{"var1": "varValue1"},
			),
			response: response{
				statusCode: http.StatusInternalServerError,
				body:       "error",
			},
		},
		"GetHTTPHandlerWithContext with pointer response": {
			handler: adapter.GetHTTPHandlerWithContext(
				func(ctx context.Context, r events.APIGatewayProxyRequest) (any, error) {
					handlerAssertions(t, &r)
					resp := mockHandlerResponse()
					return &resp, nil
				}, "/users/{userId}", map[string]string{"var1": "varValue1"}),
			response: response{
				statusCode: http.StatusOK,
				body:       "response_body",
			},
		},
		"GetHTTPHandlerWithContext without pointer response": {
			handler: adapter.GetHTTPHandlerWithContext(
				func(ctx context.Context, r events.APIGatewayProxyRequest) (any, error) {
					handlerAssertions(t, &r)
					return mockHandlerResponse(), nil
				}, "/users/{userId}", map[string]string{"var1": "varValue1"}),
			response: response{
				statusCode: http.StatusOK,
				body:       "response_body",
			},
		},
		"GetHTTPHandlerWithContext with invalid response": {
			handler: adapter.GetHTTPHandlerWithContext(
				func(ctx context.Context, r events.APIGatewayProxyRequest) (any, error) {
					return "", nil
				}, "/users/{userId}", map[string]string{"var1": "varValue1"}),
			response: response{
				statusCode: http.StatusInternalServerError,
				body:       "error",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testServer := httptest.NewServer(test.handler)
			defer testServer.Close()

			uri := testServer.URL + "/users/123?abc=123"
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, uri, strings.NewReader("req_body"))
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			res, resErr := http.DefaultClient.Do(req)
			assert.NoError(t, resErr)
			defer func() {
				assert.NoError(t, req.Body.Close())
			}()

			assert.Equal(t, test.response.statusCode, res.StatusCode)
			if test.response.statusCode == http.StatusOK {
				assert.Equal(t, []string{"single_value"}, res.Header["Single-Value-Key"])
				assert.Equal(t, []string{"multi_value_1", "multi_value_2"}, res.Header["Multi-Value-Key"])
				assert.Equal(t, []string{"single_value", "multi_value_1", "multi_value_2"}, res.Header["Mixed-Value-Key"])
			}

			body, err := io.ReadAll(res.Body)
			assert.NoError(t, err)
			assert.Equal(t, test.response.body, string(body))
		})
	}
}

func TestParseParams(t *testing.T) {
	tests := map[string]struct {
		pathPattern    string
		path           string
		expectedParams map[string]string
	}{
		"multiple params": {
			pathPattern: "/abc/{param1}/def/{param2}",
			path:        "/abc/xyz/def/123",
			expectedParams: map[string]string{
				"param1": "xyz",
				"param2": "123",
			},
		},
		"one param": {
			pathPattern: "/{name}",
			path:        "/dave",
			expectedParams: map[string]string{
				"name": "dave",
			},
		},
		"param with hyphens": {
			pathPattern: "/root/{middle}/end",
			path:        "/root/abc-def/end",
			expectedParams: map[string]string{
				"middle": "abc-def",
			},
		},
		"one param no matches": {
			pathPattern:    "/greet/{name}",
			path:           "/",
			expectedParams: map[string]string{},
		},
		"no params": {
			pathPattern:    "/users",
			path:           "/users",
			expectedParams: map[string]string{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testServer := httptest.NewServer(adapter.GetHTTPHandler(
				func(r events.APIGatewayProxyRequest) (any, error) {
					return mockHandlerResponse(), nil
				}, test.pathPattern, nil,
			))
			defer testServer.Close()

			uri := testServer.URL + test.path
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, uri, strings.NewReader("req_body"))
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			res, resErr := http.DefaultClient.Do(req)
			assert.NoError(t, resErr)
			defer func() {
				assert.NoError(t, req.Body.Close())
			}()

			assert.Equal(t, test.path, res.Request.URL.Path)
		})
	}
}

func mockHandlerResponse() events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Single-Value-Key": "single_value",
			"Mixed-Value-Key":  "single_value",
		},
		MultiValueHeaders: map[string][]string{
			"Multi-Value-Key": {"multi_value_1", "multi_value_2"},
			"Mixed-Value-Key": {"multi_value_1", "multi_value_2"},
		},
		Body: "response_body",
	}
}

func handlerAssertions(t *testing.T, r *events.APIGatewayProxyRequest) {
	assert.Equal(t, "POST", r.HTTPMethod)
	assert.Equal(t, "/users/123", r.Path)
	assert.Equal(t, "123", r.PathParameters["userId"])
	assert.Equal(t, "123", r.QueryStringParameters["abc"])
	assert.Equal(t, "123", r.MultiValueQueryStringParameters["abc"][0])
	assert.Equal(t, "application/json", r.Headers["Content-Type"])
	assert.Equal(t, "application/json", r.MultiValueHeaders["Content-Type"][0])
	assert.Equal(t, "req_body", r.Body)
	assert.Equal(t, "varValue1", r.StageVariables["var1"])
}
