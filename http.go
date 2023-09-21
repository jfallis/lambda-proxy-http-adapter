package lambdaproxyhttpadapter

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/gorilla/reverse"
)

type APIGatewayProxyHandler func(r events.APIGatewayProxyRequest) (any, error)

type APIGatewayProxyHandlerWithContext func(ctx context.Context, r events.APIGatewayProxyRequest) (any, error)

func GetHTTPHandler(
	lambdaHandler APIGatewayProxyHandler,
	resourcePathPattern string,
	stageVariables map[string]string,
	proxyRequestCtx *events.APIGatewayProxyRequestContext,
) http.Handler {
	return getHTTPHandler(
		func(ctx context.Context, r events.APIGatewayProxyRequest) (any, error) {
			return lambdaHandler(r)
		},
		resourcePathPattern,
		stageVariables,
		proxyRequestCtx,
	)
}

func GetHTTPHandlerWithContext(
	lambdaHandler APIGatewayProxyHandlerWithContext,
	resourcePathPattern string,
	stageVariables map[string]string,
	proxyRequestCtx *events.APIGatewayProxyRequestContext,
) http.Handler {
	return getHTTPHandler(lambdaHandler, resourcePathPattern, stageVariables, proxyRequestCtx)
}

func getHTTPHandler(
	lambdaHandler APIGatewayProxyHandlerWithContext,
	resourcePathPattern string,
	stageVariables map[string]string,
	proxyRequestCtx *events.APIGatewayProxyRequestContext,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		proxyResponse, err := lambdaHandler(
			r.Context(),
			APIGatewayProxyRequestAdaptor(r, string(body), resourcePathPattern, stageVariables, proxyRequestCtx),
		)

		if err != nil {
			// write a generic error, the same as API GW would if an error was returned by handler
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`error`))
			return
		}

		if wErr := writeResponse(w, proxyResponse); wErr != nil {
			// write a generic error, the same as API GW would if an error was returned by handler
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`error`))
			return
		}
	})
}

func APIGatewayProxyRequestAdaptor(
	r *http.Request,
	body,
	resourcePathPattern string,
	stageVariables map[string]string,
	requestContext *events.APIGatewayProxyRequestContext,
) events.APIGatewayProxyRequest {
	if requestContext == nil {
		requestContext = new(events.APIGatewayProxyRequestContext)
	}
	return events.APIGatewayProxyRequest{
		Resource:                        resourcePathPattern,
		Path:                            r.URL.Path,
		HTTPMethod:                      r.Method,
		Headers:                         singleValue(r.Header),
		MultiValueHeaders:               r.Header,
		QueryStringParameters:           singleValue(r.URL.Query()),
		MultiValueQueryStringParameters: r.URL.Query(),
		PathParameters:                  parsePathParams(resourcePathPattern, r.URL.Path),
		StageVariables:                  stageVariables,
		RequestContext:                  *requestContext,
		Body:                            body,
	}
}

func singleValue(multiValueMap map[string][]string) map[string]string {
	singleValueMap := make(map[string]string)
	for k, mv := range multiValueMap {
		if len(mv) > 0 {
			singleValueMap[k] = mv[0]
		}
	}
	return singleValueMap
}

func parsePathParams(pathPattern, path string) map[string]string {
	re, err := reverse.NewGorillaPath(pathPattern, false)
	if err != nil {
		return map[string]string{}
	}

	params := make(map[string]string)
	if matches := re.MatchString(path); matches {
		for name, values := range re.Values(path) {
			if len(values) > 0 {
				params[name] = values[0]
			}
		}
	}

	return params
}

func writeResponse(w http.ResponseWriter, proxyResponse any) error {
	r, _ := proxyResponse.(*events.APIGatewayProxyResponse)
	if p, ok := proxyResponse.(events.APIGatewayProxyResponse); ok {
		r = &p
	}
	if r == nil {
		return errors.New("proxy response is nil")
	}

	for k, v := range r.Headers {
		w.Header().Add(k, v)
	}

	for k, vs := range r.MultiValueHeaders {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(r.StatusCode)
	_, _ = w.Write([]byte(r.Body))

	return nil
}
