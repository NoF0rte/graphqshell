package graphql

import (
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

type jsonWrapper struct {
	Name      string                 `json:"operationName"`
	Variables map[string]interface{} `json:"variables"`
	Query     string                 `json:"query"`
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

func (c *Client) PostJSON(obj *Object) (string, *http.Response, error) {
	query, err := obj.ToGraphQL()
	if err != nil {
		return "", nil, err
	}

	return c.postJSON(&jsonWrapper{
		Name:      obj.Name,
		Variables: make(map[string]interface{}),
		Query:     query,
	})
}

func (c *Client) PostGraphQL(obj *Object) (string, *http.Response, error) {
	query, err := obj.ToGraphQL()
	if err != nil {
		return "", nil, err
	}

	req, err := c.newRequest(c.url, http.MethodPost, graphqlMIME, query)
	if err != nil {
		return "", nil, err
	}

	return c.do(req)
}

func (c *Client) postJSON(wrapper *jsonWrapper) (string, *http.Response, error) {
	bytes, _ := json.Marshal(wrapper)
	data := string(bytes)

	req, err := c.newRequest(c.url, http.MethodPost, jsonMIME, data)
	if err != nil {
		return "", nil, err
	}

	return c.do(req)
}

func (c *Client) do(req *http.Request) (string, *http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return "", resp, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp, err
	}

	return string(body), resp, nil
}

func (c *Client) IntrospectAndParse() (*RootQuery, *RootMutation, error) {
	introspection, err := c.Introspect()
	if err != nil {
		return nil, nil, err
	}

	return Parse(introspection)
}

func (c *Client) IntrospectRaw() (string, *http.Response, error) {
	return c.postJSON(&jsonWrapper{
		Name:      "IntrospectionQuery",
		Variables: make(map[string]interface{}),
		Query:     static.IntrospectionQuery,
	})
}

func (c *Client) Introspect() (IntrospectionResponse, error) {
	var introspection IntrospectionResponse

	body, _, err := c.IntrospectRaw()
	if err != nil {
		return introspection, err
	}

	err = json.Unmarshal([]byte(body), &introspection)
	if err != nil {
		return introspection, err
	}

	return introspection, nil
}
