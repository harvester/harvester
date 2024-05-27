package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/manicminer/hamilton/auth"
	"github.com/manicminer/hamilton/environments"
	"github.com/manicminer/hamilton/odata"
)

type ApiVersion string

const (
	Version10   ApiVersion = "v1.0"
	VersionBeta ApiVersion = "beta"
)

// ConsistencyFailureFunc is a function that determines whether an HTTP request has failed due to eventual consistency and should be retried
type ConsistencyFailureFunc func(*http.Response, *odata.OData) bool

// RequestMiddleware can manipulate or log a request before it is sent
type RequestMiddleware func(*http.Request) (*http.Request, error)

// ResponseMiddleware can manipulate or log a response before it is parsed and returned
type ResponseMiddleware func(*http.Request, *http.Response) (*http.Response, error)

// RetryOn404ConsistencyFailureFunc can be used to retry a request when a 404 response is received
func RetryOn404ConsistencyFailureFunc(resp *http.Response, _ *odata.OData) bool {
	return resp != nil && resp.StatusCode == http.StatusNotFound
}

// ValidStatusFunc is a function that tests whether an HTTP response is considered valid for the particular request.
type ValidStatusFunc func(*http.Response, *odata.OData) bool

// HttpRequestInput is any type that can validate the response to an HTTP request.
type HttpRequestInput interface {
	GetConsistencyFailureFunc() ConsistencyFailureFunc
	GetContentType() string
	GetOData() odata.Query
	GetValidStatusCodes() []int
	GetValidStatusFunc() ValidStatusFunc
}

// Uri represents a Microsoft Graph endpoint.
type Uri struct {
	Entity      string
	Params      url.Values
	HasTenantId bool
}

// RetryableErrorHandler ensures that the response is returned after exhausting retries for a request
// We can't return an error here, or net/http will not return the response
func RetryableErrorHandler(resp *http.Response, err error, numTries int) (*http.Response, error) {
	return resp, nil
}

// Client is a base client to be used by clients for specific entities.
// It can send GET, POST, PUT, PATCH and DELETE requests to Microsoft Graph and is API version and tenant aware.
type Client struct {
	// Endpoint is the base endpoint for Microsoft Graph, usually "https://graph.microsoft.com".
	Endpoint environments.ApiEndpoint

	// ApiVersion is the Microsoft Graph API version to use.
	ApiVersion ApiVersion

	// TenantId is the tenant ID to use in requests.
	TenantId string

	// UserAgent is the HTTP user agent string to send in requests.
	UserAgent string

	// Authorizer is anything that can provide an access token with which to authorize requests.
	Authorizer auth.Authorizer

	// DisableRetries prevents the client from reattempting failed requests (which it does to work around eventual consistency issues).
	// This does not impact handling of retries related to rate limiting, which are always performed.
	DisableRetries bool

	// RequestMiddlewares is a slice of functions that are called in order before a request is sent
	RequestMiddlewares *[]RequestMiddleware

	// ResponseMiddlewares is a slice of functions that are called in order before a response is parsed and returned
	ResponseMiddlewares *[]ResponseMiddleware

	// HttpClient is the underlying http.Client, which by default uses a retryable client
	HttpClient      *http.Client
	RetryableClient *retryablehttp.Client
}

// NewClient returns a new Client configured with the specified API version and tenant ID.
func NewClient(apiVersion ApiVersion, tenantId string) Client {
	r := retryablehttp.NewClient()
	r.ErrorHandler = RetryableErrorHandler
	r.Logger = nil

	return Client{
		Endpoint:        environments.MsGraphGlobal.Endpoint,
		ApiVersion:      apiVersion,
		TenantId:        tenantId,
		UserAgent:       "Hamilton (Go-http-client/1.1)",
		HttpClient:      r.StandardClient(),
		RetryableClient: r,
	}
}

// buildUri is used by the package to build a complete URI string for API requests.
func (c Client) buildUri(uri Uri) (string, error) {
	newUrl, err := url.Parse(string(c.Endpoint))
	if err != nil {
		return "", err
	}
	newUrl.Path = "/" + string(c.ApiVersion)
	if uri.HasTenantId {
		newUrl.Path = fmt.Sprintf("%s/%s", newUrl.Path, c.TenantId)
	}
	newUrl.Path = fmt.Sprintf("%s/%s", newUrl.Path, strings.TrimLeft(uri.Entity, "/"))
	if uri.Params != nil {
		newUrl.RawQuery = uri.Params.Encode()
	}
	return newUrl.String(), nil
}

// performRequest is used by the package to send an HTTP request to the API.
func (c Client) performRequest(req *http.Request, input HttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int

	query := input.GetOData()
	req.Header = query.AppendHeaders(req.Header)
	req.Header.Add("Content-Type", input.GetContentType())

	if c.Authorizer != nil {
		token, err := c.Authorizer.Token()
		if err != nil {
			return nil, status, nil, err
		}
		token.SetAuthHeader(req)
	}

	if c.UserAgent != "" {
		req.Header.Add("User-Agent", c.UserAgent)
	}

	var resp *http.Response
	var o *odata.OData
	var err error

	var reqBody []byte
	if req.Body != nil {
		reqBody, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, status, nil, fmt.Errorf("reading request body: %v", err)
		}
	}

	c.RetryableClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && !c.DisableRetries {
			if resp.StatusCode == http.StatusFailedDependency {
				return true, nil
			}

			o, err = odata.FromResponse(resp)
			if err != nil {
				return false, err
			}

			f := input.GetConsistencyFailureFunc()
			if f != nil && f(resp, o) {
				return true, nil
			}
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	req.Body = io.NopCloser(bytes.NewBuffer(reqBody))

	if c.RequestMiddlewares != nil {
		for _, m := range *c.RequestMiddlewares {
			r, err := m(req)
			if err != nil {
				return nil, status, nil, err
			}
			req = r
		}
	}

	resp, err = c.HttpClient.Do(req)
	if err != nil {
		return nil, status, nil, err
	}

	if c.ResponseMiddlewares != nil {
		for _, m := range *c.ResponseMiddlewares {
			r, err := m(req, resp)
			if err != nil {
				return nil, status, nil, err
			}
			resp = r
		}
	}

	o, err = odata.FromResponse(resp)
	if err != nil {
		return nil, status, o, err
	}
	if resp == nil {
		return resp, status, o, fmt.Errorf("nil response received")
	}

	status = resp.StatusCode
	if !containsStatusCode(input.GetValidStatusCodes(), status) {
		f := input.GetValidStatusFunc()
		if f != nil && f(resp, o) {
			return resp, status, o, nil
		}

		var errText string
		switch {
		case o != nil && o.Error != nil && o.Error.String() != "":
			errText = fmt.Sprintf("OData error: %s", o.Error)
		default:
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, status, o, fmt.Errorf("unexpected status %d, could not read response body", status)
			}
			if len(respBody) == 0 {
				return nil, status, o, fmt.Errorf("unexpected status %d received with no body", status)
			}
			errText = fmt.Sprintf("response: %s", respBody)
		}
		return nil, status, o, fmt.Errorf("unexpected status %d with %s", status, errText)
	}

	return resp, status, o, nil
}

// containsStatusCode determines whether the returned status code is in the []int of expected status codes.
func containsStatusCode(expected []int, actual int) bool {
	for _, v := range expected {
		if actual == v {
			return true
		}
	}

	return false
}

// DeleteHttpRequestInput configures a DELETE request.
type DeleteHttpRequestInput struct {
	ConsistencyFailureFunc ConsistencyFailureFunc
	OData                  odata.Query
	ValidStatusCodes       []int
	ValidStatusFunc        ValidStatusFunc
	Uri                    Uri
}

// GetConsistencyFailureFunc returns a function used to evaluate whether a failed request is due to eventual consistency and should be retried.
func (i DeleteHttpRequestInput) GetConsistencyFailureFunc() ConsistencyFailureFunc {
	return i.ConsistencyFailureFunc
}

// GetContentType returns the content type for the request, currently only application/json is supported
func (i DeleteHttpRequestInput) GetContentType() string {
	return "application/json; charset=utf-8"
}

// GetOData returns the OData request metadata
func (i DeleteHttpRequestInput) GetOData() odata.Query {
	return i.OData
}

// GetValidStatusCodes returns a []int of status codes considered valid for a DELETE request.
func (i DeleteHttpRequestInput) GetValidStatusCodes() []int {
	return i.ValidStatusCodes
}

// GetValidStatusFunc returns a function used to evaluate whether the response to a DELETE request is considered valid.
func (i DeleteHttpRequestInput) GetValidStatusFunc() ValidStatusFunc {
	return i.ValidStatusFunc
}

// Delete performs a DELETE request.
func (c Client) Delete(ctx context.Context, input DeleteHttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int
	url, err := c.buildUri(input.Uri)
	if err != nil {
		return nil, status, nil, fmt.Errorf("unable to make request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, http.NoBody)
	if err != nil {
		return nil, status, nil, err
	}
	resp, status, o, err := c.performRequest(req, input)
	if err != nil {
		return nil, status, o, err
	}
	return resp, status, o, nil
}

// GetHttpRequestInput configures a GET request.
type GetHttpRequestInput struct {
	ConsistencyFailureFunc ConsistencyFailureFunc
	DisablePaging          bool
	OData                  odata.Query
	ValidStatusCodes       []int
	ValidStatusFunc        ValidStatusFunc
	Uri                    Uri
	rawUri                 string
}

// GetConsistencyFailureFunc returns a function used to evaluate whether a failed request is due to eventual consistency and should be retried.
func (i GetHttpRequestInput) GetConsistencyFailureFunc() ConsistencyFailureFunc {
	return i.ConsistencyFailureFunc
}

// GetContentType returns the content type for the request, currently only application/json is supported
func (i GetHttpRequestInput) GetContentType() string {
	return "application/json; charset=utf-8"
}

// GetOData returns the OData request metadata
func (i GetHttpRequestInput) GetOData() odata.Query {
	return i.OData
}

// GetValidStatusCodes returns a []int of status codes considered valid for a GET request.
func (i GetHttpRequestInput) GetValidStatusCodes() []int {
	return i.ValidStatusCodes
}

// GetValidStatusFunc returns a function used to evaluate whether the response to a GET request is considered valid.
func (i GetHttpRequestInput) GetValidStatusFunc() ValidStatusFunc {
	return i.ValidStatusFunc
}

// Get performs a GET request.
func (c Client) Get(ctx context.Context, input GetHttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int

	// Check for a raw uri, else build one from the Uri field
	url := input.rawUri
	if url == "" {
		// Append odata query parameters
		input.Uri.Params = input.OData.AppendValues(input.Uri.Params)

		var err error
		url, err = c.buildUri(input.Uri)
		if err != nil {
			return nil, status, nil, fmt.Errorf("unable to make request: %v", err)
		}
	}

	// Build a new request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, status, nil, err
	}

	// Perform the request
	resp, status, o, err := c.performRequest(req, input)
	if err != nil {
		return nil, status, o, err
	}

	// Check for json content before handling pagination
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/json") {
		// Read the response body and close it
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, status, o, fmt.Errorf("could not parse response body")
		}
		resp.Body.Close()

		// Unmarshall firstOdata
		var firstOdata odata.OData
		if err := json.Unmarshal(respBody, &firstOdata); err != nil {
			return nil, status, o, err
		}

		firstValue, ok := firstOdata.Value.([]interface{})
		if input.DisablePaging || firstOdata.NextLink == nil || firstValue == nil || !ok {
			// No more pages, reassign response body and return
			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
			return resp, status, o, nil
		}

		// Get the next page, recursively
		nextInput := input
		nextInput.rawUri = *firstOdata.NextLink
		nextResp, status, o, err := c.Get(ctx, nextInput)
		if err != nil {
			return resp, status, o, err
		}

		// Read the next page response body and close it
		nextRespBody, err := io.ReadAll(nextResp.Body)
		if err != nil {
			return nil, status, o, fmt.Errorf("could not parse response body")
		}
		nextResp.Body.Close()

		// Unmarshall firstOdata from the next page
		var nextOdata odata.OData
		if err := json.Unmarshal(nextRespBody, &nextOdata); err != nil {
			return resp, status, o, err
		}

		if nextValue, ok := nextOdata.Value.([]interface{}); ok {
			// Next page has results, append to current page
			value := append(firstValue, nextValue...)
			nextOdata.Value = &value
		}

		// Marshal the entire result, along with fields from the final page
		newJson, err := json.Marshal(nextOdata)
		if err != nil {
			return resp, status, o, err
		}

		// Reassign the response body
		resp.Body = io.NopCloser(bytes.NewBuffer(newJson))
	}

	return resp, status, o, nil
}

// PatchHttpRequestInput configures a PATCH request.
type PatchHttpRequestInput struct {
	ConsistencyFailureFunc ConsistencyFailureFunc
	Body                   []byte
	OData                  odata.Query
	ValidStatusCodes       []int
	ValidStatusFunc        ValidStatusFunc
	Uri                    Uri
}

// GetConsistencyFailureFunc returns a function used to evaluate whether a failed request is due to eventual consistency and should be retried.
func (i PatchHttpRequestInput) GetConsistencyFailureFunc() ConsistencyFailureFunc {
	return i.ConsistencyFailureFunc
}

// GetContentType returns the content type for the request, currently only application/json is supported
func (i PatchHttpRequestInput) GetContentType() string {
	return "application/json; charset=utf-8"
}

// GetOData returns the OData request metadata
func (i PatchHttpRequestInput) GetOData() odata.Query {
	return i.OData
}

// GetValidStatusCodes returns a []int of status codes considered valid for a PATCH request.
func (i PatchHttpRequestInput) GetValidStatusCodes() []int {
	return i.ValidStatusCodes
}

// GetValidStatusFunc returns a function used to evaluate whether the response to a PATCH request is considered valid.
func (i PatchHttpRequestInput) GetValidStatusFunc() ValidStatusFunc {
	return i.ValidStatusFunc
}

// Patch performs a PATCH request.
func (c Client) Patch(ctx context.Context, input PatchHttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int
	url, err := c.buildUri(input.Uri)
	if err != nil {
		return nil, status, nil, fmt.Errorf("unable to make request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(input.Body))
	if err != nil {
		return nil, status, nil, err
	}
	resp, status, o, err := c.performRequest(req, input)
	if err != nil {
		return nil, status, o, err
	}
	return resp, status, o, nil
}

// PostHttpRequestInput configures a POST request.
type PostHttpRequestInput struct {
	Body                   []byte
	ConsistencyFailureFunc ConsistencyFailureFunc
	OData                  odata.Query
	ValidStatusCodes       []int
	ValidStatusFunc        ValidStatusFunc
	Uri                    Uri
}

// GetConsistencyFailureFunc returns a function used to evaluate whether a failed request is due to eventual consistency and should be retried.
func (i PostHttpRequestInput) GetConsistencyFailureFunc() ConsistencyFailureFunc {
	return i.ConsistencyFailureFunc
}

// GetContentType returns the content type for the request, currently only application/json is supported
func (i PostHttpRequestInput) GetContentType() string {
	return "application/json; charset=utf-8"
}

// GetOData returns the OData request metadata
func (i PostHttpRequestInput) GetOData() odata.Query {
	return i.OData
}

// GetValidStatusCodes returns a []int of status codes considered valid for a POST request.
func (i PostHttpRequestInput) GetValidStatusCodes() []int {
	return i.ValidStatusCodes
}

// GetValidStatusFunc returns a function used to evaluate whether the response to a POST request is considered valid.
func (i PostHttpRequestInput) GetValidStatusFunc() ValidStatusFunc {
	return i.ValidStatusFunc
}

// Post performs a POST request.
func (c Client) Post(ctx context.Context, input PostHttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int
	url, err := c.buildUri(input.Uri)
	if err != nil {
		return nil, status, nil, fmt.Errorf("unable to make request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(input.Body))
	if err != nil {
		return nil, status, nil, err
	}
	resp, status, o, err := c.performRequest(req, input)
	if err != nil {
		return nil, status, o, err
	}
	return resp, status, o, nil
}

// PutHttpRequestInput configures a PUT request.
type PutHttpRequestInput struct {
	ConsistencyFailureFunc ConsistencyFailureFunc
	ContentType            string
	Body                   []byte
	OData                  odata.Query
	ValidStatusCodes       []int
	ValidStatusFunc        ValidStatusFunc
	Uri                    Uri
}

// GetConsistencyFailureFunc returns a function used to evaluate whether a failed request is due to eventual consistency and should be retried.
func (i PutHttpRequestInput) GetConsistencyFailureFunc() ConsistencyFailureFunc {
	return i.ConsistencyFailureFunc
}

// GetContentType returns the content type for the request, defaults to application/json
func (i PutHttpRequestInput) GetContentType() string {
	if i.ContentType != "" {
		return i.ContentType
	}
	return "application/json; charset=utf-8"
}

// GetOData returns the OData request metadata
func (i PutHttpRequestInput) GetOData() odata.Query {
	return i.OData
}

// GetValidStatusCodes returns a []int of status codes considered valid for a PUT request.
func (i PutHttpRequestInput) GetValidStatusCodes() []int {
	return i.ValidStatusCodes
}

// GetValidStatusFunc returns a function used to evaluate whether the response to a PUT request is considered valid.
func (i PutHttpRequestInput) GetValidStatusFunc() ValidStatusFunc {
	return i.ValidStatusFunc
}

// Put performs a PUT request.
func (c Client) Put(ctx context.Context, input PutHttpRequestInput) (*http.Response, int, *odata.OData, error) {
	var status int
	url, err := c.buildUri(input.Uri)
	if err != nil {
		return nil, status, nil, fmt.Errorf("unable to make request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(input.Body))
	if err != nil {
		return nil, status, nil, err
	}
	resp, status, o, err := c.performRequest(req, input)
	if err != nil {
		return nil, status, o, err
	}
	return resp, status, o, nil
}
