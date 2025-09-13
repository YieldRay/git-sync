// Bitbucket Cloud REST API
// Overview (v1): https://support.atlassian.com/bitbucket-cloud/docs/use-bitbucket-rest-api-version-1/
// Overview (v2): https://support.atlassian.com/bitbucket-cloud/docs/use-the-bitbucket-cloud-rest-apis/
// Repositories (v2 group): https://developer.atlassian.com/cloud/bitbucket/rest/api-group-repositories/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type BitbucketRepo struct {
	UUID      string `json:"uuid"`
	Slug      string `json:"slug"`
	IsPrivate bool   `json:"is_private"`
	Links     struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
		Clone []struct {
			Href string `json:"href"`
			Name string `json:"name"`
		} `json:"clone"`
	} `json:"links"`
}

// doBitbucketRequest builds a request against the Bitbucket v2 API (https://api.bitbucket.org/2.0)
// and authenticates using Basic Auth with a username + App Password.
// API tokens: https://support.atlassian.com/bitbucket-cloud/docs/api-tokens/
func doBitbucketRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	// Build URL manually to handle pre-encoded paths properly
	baseURL := "https://api.bitbucket.org/2.0" + path
	if len(queryParams) > 0 {
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		baseURL = u.String()
	}

	req, err := http.NewRequest(method, baseURL, body)
	if err != nil {
		return nil, err
	}
	// https://developer.atlassian.com/cloud/bitbucket/rest/intro/#authentication
	// Use email + API token for authentication (not username + app password)
	req.SetBasicAuth(config.BitbucketEmail, config.BitbucketToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return bbClient.Do(req)
}

func handleBitbucketResponse(resp *http.Response, target any) (any, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if target == nil {
			return nil, nil
		}
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return nil, err
		}
		return target, nil
	}
	b, _ := io.ReadAll(resp.Body)
	log.Printf("Bitbucket API error %d: %s", resp.StatusCode, string(b))
	return nil, fmt.Errorf("API error")
}

// GET repository
// Docs: https://developer.atlassian.com/cloud/bitbucket/rest/api-group-repositories/#api-repositories-workspace-repo-slug-get
func getBitbucketRepo(workspace, repoSlug string) (*BitbucketRepo, error) {
	resp, err := doBitbucketRequest("GET", fmt.Sprintf("/repositories/%s/%s", workspace, repoSlug), nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, nil
	}
	var repo BitbucketRepo
	result, err := handleBitbucketResponse(resp, &repo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*BitbucketRepo), nil
}

// CREATE repository
// Docs: https://developer.atlassian.com/cloud/bitbucket/rest/api-group-repositories/#api-repositories-workspace-repo-slug-post
func createBitbucketRepo(workspace, repoSlug string, private bool) (*BitbucketRepo, error) {
	body := map[string]any{
		"scm":        "git",
		"is_private": private,
	}
	byts, _ := json.Marshal(body)
	resp, err := doBitbucketRequest("POST", fmt.Sprintf("/repositories/%s/%s", workspace, repoSlug), nil, bytes.NewReader(byts))
	if err != nil {
		return nil, err
	}
	var repo BitbucketRepo
	result, err := handleBitbucketResponse(resp, &repo)
	if err != nil {
		return nil, err
	}
	if result != nil {
		log.Printf("Created Bitbucket repo %s/%s", workspace, repoSlug)
		return result.(*BitbucketRepo), nil
	}
	return nil, fmt.Errorf("unexpected response")
}

// UPDATE repository (toggle privacy via is_private)
// Docs: https://developer.atlassian.com/cloud/bitbucket/rest/api-group-repositories/#api-repositories-workspace-repo-slug-put
func updateBitbucketRepoPrivacy(workspace, repoSlug string, private bool) (*BitbucketRepo, error) {
	body := map[string]any{"is_private": private}
	byts, _ := json.Marshal(body)
	resp, err := doBitbucketRequest("PUT", fmt.Sprintf("/repositories/%s/%s", workspace, repoSlug), nil, bytes.NewReader(byts))
	if err != nil {
		return nil, err
	}
	var repo BitbucketRepo
	result, err := handleBitbucketResponse(resp, &repo)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result.(*BitbucketRepo), nil
	}
	return nil, fmt.Errorf("unexpected response")
}

// Ensure repository exists and matches desired privacy; create or update as needed.
func checkAndValidateBitbucketRepo(workspace, repoSlug string, private bool) error {
	repo, err := getBitbucketRepo(workspace, repoSlug)
	if err != nil {
		return err
	}
	if repo == nil {
		_, err := createBitbucketRepo(workspace, repoSlug, private)
		return err
	}
	if repo.IsPrivate != private {
		_, err := updateBitbucketRepoPrivacy(workspace, repoSlug, private)
		return err
	}
	log.Printf("Bitbucket repo %s/%s exists with desired privacy %v", workspace, repoSlug, private)
	return nil
}

// Push a mirrored repository to Bitbucket over HTTPS with API Token.
// https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/
func syncToBitbucket(email, token, workspace, repoSlug, localPath string) error {
	bbURL := fmt.Sprintf("https://bitbucket.org/%s/%s.git", workspace, repoSlug)
	pushURL := strings.Replace(bbURL, "https://", fmt.Sprintf("https://x-bitbucket-api-token-auth:%s@", token), 1)
	log.Printf("Pushing %s -> Bitbucket (%s) ...", repoSlug, workspace)
	return runCmd("git", "--git-dir", localPath, "push", "--mirror", pushURL)
}
