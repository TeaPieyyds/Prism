package g79client

import "net/http"

// NewClientWithHTTPClient creates a client and overrides its HTTP client.
func NewClientWithHTTPClient(httpClient *http.Client) (*Client, error) {
	c, err := NewClient()
	if err != nil {
		return nil, err
	}
	if httpClient != nil {
		c.httpClient = httpClient
	}
	return c, nil
}
