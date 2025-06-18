// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package util

import (
	"log"
	"net/http"
	"net/url"
)

type HttpTransport struct {
	BaseTransport http.RoundTripper
	BaseUrl       string
	Headers       map[string]string
}

func (t *HttpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}

	baseUrl, err := url.Parse(t.BaseUrl)
	if err != nil {
		log.Fatal(err.Error())
		return nil, err
	}
	req.URL = baseUrl.ResolveReference(req.URL)

	baseTransport := t.BaseTransport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	return baseTransport.RoundTrip(req)
}
