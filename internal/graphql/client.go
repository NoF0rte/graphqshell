package graphql

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/NoF0rte/graphqshell/internal/static"
)

const (
	jsonMIME    string = "application/json"
	graphqlMIME string = "application/graphql"
)

type Request struct {
	Name      string                 `json:"operationName"`
	Variables map[string]interface{} `json:"variables"`
	Query     string                 `json:"query"`
}

type Result struct {
	Data   map[string]interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Raw string `json:"-"`
}

type Response struct {
	Result      *Result
	RawResponse *http.Response
}

type ClientOptions struct {
	headers     map[string]string
	proxy       string
	contentType string
}
type ClientOption func(o *ClientOptions)

type Client struct {
	url     string
	http    *http.Client
	options *ClientOptions
}

func WithHeaders(headers map[string]string) ClientOption {
	return func(o *ClientOptions) {
		for key, value := range headers {
			o.headers[key] = value
		}
	}
}

func WithCookies(cookies string) ClientOption {
	return func(o *ClientOptions) {
		o.headers["Cookie"] = cookies
	}
}

func WithAuthorization(value string) ClientOption {
	return func(o *ClientOptions) {
		o.headers["Authorization"] = value
	}
}

func WithBearerToken(token string) ClientOption {
	return WithAuthorization(fmt.Sprintf("Bearer %s", token))
}

func WithProxy(proxyURL string) ClientOption {
	return func(o *ClientOptions) {
		o.proxy = proxyURL
	}
}

// func WithDefaultContentType(contentType string) ClientOption {
// 	return func(o *ClientOptions) {
// 		o.contentType = contentType
// 	}
// }

func NewClient(graphqlURL string, opts ...ClientOption) *Client {
	options := &ClientOptions{
		headers: make(map[string]string),
	}
	for _, opt := range opts {
		opt(options)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	if options.proxy != "" {
		proxyURL, _ := url.Parse(options.proxy)
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &Client{
		url: graphqlURL,
		http: &http.Client{
			Transport: transport,
		},
		options: options,
	}
}

func (c *Client) SetHeaders(headers map[string]string) {
	for key, value := range headers {
		c.options.headers[key] = value
	}
}

func (c *Client) GetHeaders() map[string]string {
	return c.options.headers
}

func (c *Client) SetCookies(cookies string) {
	c.options.headers["Cookie"] = cookies
}

func (c *Client) GetCookies() string {
	return c.options.headers["Cookie"]
}

func (c *Client) SetAuth(value string) {
	c.options.headers["Authorization"] = value
}

func (c *Client) GetAuth() string {
	return c.options.headers["Authorization"]
}

func (c *Client) SetBearer(token string) {
	c.SetAuth(fmt.Sprintf("Bearer %s", token))
}

func (c *Client) GetBearer() string {
	auth := c.GetAuth()
	if auth == "" {
		return ""
	}

	parts := strings.Split(auth, "Bearer ")
	if len(parts) == 1 {
		return parts[0]
	}

	return parts[1]
}

func (c *Client) SetProxy(proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}

	c.options.proxy = proxyURL

	c.http.Transport.(*http.Transport).Proxy = http.ProxyURL(u)

	return nil
}

func (c *Client) GetProxy() string {
	return c.options.proxy
}

func (c *Client) RemoveProxy() {
	c.http.Transport.(*http.Transport).Proxy = nil
	c.options.proxy = ""
}

func (c *Client) newRequest(url string, method string, contentType string, data interface{}) (*http.Request, error) {
	var body io.Reader
	if data != nil {
		body = strings.NewReader(data.(string))
	}

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for key, value := range c.options.headers {
		request.Header.Add(key, value)
	}

	request.Header.Add("Content-Type", contentType)

	return request, nil
}

func (c *Client) PostJSON(obj *Object, vars ...*Variable) (*Response, error) {
	query, err := obj.ToGraphQL(vars...)
	if err != nil {
		return nil, err
	}

	variables := make(map[string]interface{})
	for _, v := range vars {
		variables[v.Name] = v.Value
	}

	return c.Post(&Request{
		Name:      obj.Name,
		Variables: variables,
		Query:     query,
	})
}

func (c *Client) PostGraphQL(obj *Object) (*Response, error) {
	query, err := obj.ToGraphQL()
	if err != nil {
		return nil, err
	}

	return c.post(query, graphqlMIME)
}

func (c *Client) Post(request *Request) (*Response, error) {
	bytes, _ := json.Marshal(request)
	data := string(bytes)

	return c.post(data, jsonMIME)
}

func (c *Client) post(body string, contentType string) (*Response, error) {
	req, err := c.newRequest(c.url, http.MethodPost, contentType, body)
	if err != nil {
		return nil, err
	}

	return c.do(req)
}

func (c *Client) do(req *http.Request) (*Response, error) {
	resp, err := c.http.Do(req)
	response := &Response{
		RawResponse: resp,
		Result:      &Result{},
	}

	if err != nil {
		return response, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	if err != nil && (!strings.Contains(err.Error(), "remote error") || c.options.proxy == "") {
		return response, err
	}

	err = json.Unmarshal(body, response.Result)
	if err != nil {
		return response, err
	}

	response.Result.Raw = string(body)
	return response, nil
}

func (c *Client) IntrospectAndParse() (*RootQuery, *RootMutation, error) {
	introspection, err := c.Introspect()
	if err != nil {
		return nil, nil, err
	}

	return Parse(introspection)
}

func (c *Client) IntrospectRaw() (*Response, error) {
	return c.Post(&Request{
		Name:      "IntrospectionQuery",
		Variables: make(map[string]interface{}),
		Query:     static.IntrospectionQuery,
	})
}

func (c *Client) Introspect() (IntrospectionResponse, error) {
	var introspection IntrospectionResponse

	resp, err := c.IntrospectRaw()
	if err != nil {
		return introspection, err
	}

	err = json.Unmarshal([]byte(resp.Result.Raw), &introspection)
	if err != nil {
		return introspection, err
	}

	return introspection, nil
}
