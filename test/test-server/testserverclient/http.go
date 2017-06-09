package testserverclient

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type httpClient struct {
	client  *http.Client
	baseURL string
	method  string
	path    string
	header  map[string]string
}

type Response struct {
	Status         int         `json:"-"`
	ResponseHeader http.Header `json:"-"`
	Type           string      `json:"type,omitempty"`
	PodName        string      `json:"podName,omitempty"`
	Host           string      `json:"host,omitempty"`
	ServerPort     string      `json:"serverPort,omitempty"`
	Path           string      `json:"path,omitempty"`
	Method         string      `json:"method,omitempty"`
	RequestHeaders http.Header `json:"headers,omitempty"`
	Body           string      `json:"body,omitempty"`
}

func NewTestHTTPClient(url string) *httpClient {
	url = strings.TrimSuffix(url, "/")
	return &httpClient{
		client:  &http.Client{Timeout: time.Second * 5},
		baseURL: url,
	}
}

func (t *httpClient) Method(method string) *httpClient {
	t.method = method
	return t
}

func (t *httpClient) Path(path string) *httpClient {
	path = strings.TrimPrefix(path, "/")
	t.path = path
	return t
}

func (t *httpClient) Header(h map[string]string) *httpClient {
	t.header = h
	return t
}

func (t *httpClient) DoWithRetry(limit int) (*Response, error) {
	var resp *Response
	var err error
	for i := 1; i <= limit; i++ {
		resp, err = t.do(true)
		if err == nil {
			return resp, err
		}
		time.Sleep(time.Second * 5)
	}
	return resp, err
}

func (t *httpClient) DoTestRedirectWithRetry(limit int) (*Response, error) {
	var resp *Response
	var err error

	// Do Not redirect
	t.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for i := 1; i <= limit; i++ {
		resp, err = t.do(false)
		if err == nil {
			return resp, err
		}
		time.Sleep(time.Second * 5)
	}
	return resp, err
}

func (t *httpClient) DoStatusWithRetry(limit int) (*Response, error) {
	var resp *Response
	var err error
	for i := 1; i <= limit; i++ {
		resp, err = t.do(false)
		if err == nil {
			return resp, err
		}
		time.Sleep(time.Second * 5)
	}
	return resp, err
}

func (t *httpClient) do(parse bool) (*Response, error) {
	req, err := http.NewRequest(t.method, t.baseURL+"/"+t.path, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range t.header {
		req.Header.Add(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}

	responseStruct := &Response{
		Status:         resp.StatusCode,
		ResponseHeader: resp.Header,
	}
	if parse {
		err = json.NewDecoder(resp.Body).Decode(responseStruct)
		if err != nil {
			return nil, err
		}
	}
	return responseStruct, nil
}
