package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
)

func NewRoundTripper(roundTrip func(req *http.Request) (*http.Response, error)) http.RoundTripper {
	return transportRoundTrip{
		RoundTripImpl: roundTrip,
	}
}

// this type is just a wrapper for the interface: RoundTrip
type transportRoundTrip struct {
	RoundTripImpl func(req *http.Request) (*http.Response, error)
}

func (trt transportRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return trt.RoundTripImpl(req)
}

// transport is a custom RoundTripper that logs request/response details
// transport wraps DefaultTransport to log request/response details
var transport = NewRoundTripper(func(req *http.Request) (*http.Response, error) {
	now := time.Now()
	var err error

	var reqAllBody []byte
	if req.Body != nil {
		if reqAllBody, err = io.ReadAll(req.Body); err != nil {
			log.Printf("❌ Error reading request body: %v", err)
		} else {
			req.Body = io.NopCloser(bytes.NewReader(reqAllBody)) // clone body
		}
	}
	// capture request headers if needed (not currently used)

	// Perform HTTP request using default transport
	res, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		log.Printf("❌ Error performing request: %v", err)
		return res, err
	}

	var resAllBody []byte
	if res.Body != nil {
		if resAllBody, err = io.ReadAll(res.Body); err != nil {
			log.Printf("❌ Error reading response body: %v", err)
		} else {
			res.Body = io.NopCloser(bytes.NewReader(resAllBody)) // clone body
		}
	}
	// capture response headers if needed (not currently used)
	// var resHeaders map[string][]string = res.Header.Clone()

	// Log duration and status
	log.Printf("📡 %s %s -> %d (%v)", req.Method, req.URL, res.StatusCode, time.Since(now))
	return res, err
})
