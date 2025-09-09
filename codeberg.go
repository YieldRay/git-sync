// https://codeberg.org/user/settings/applications
package main

import (
	"io"
	"net/http"
	"net/url"
)

// https://forgejo.org/docs/latest/user/api-usage/\
// https://codeberg.org/api/swagger
// https://scalar.val.run/codeberg.org/swagger.v1.json
func doCodebergRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse("https://codeberg.com")
	if err != nil {
		return nil, err
	}
	u.Path = path
	q := u.Query()
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+config.CodebergToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return glClient.Do(req)
}
