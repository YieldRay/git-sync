// https://codeberg.org/user/settings/applications
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

// https://forgejo.org/docs/latest/user/api-usage/
// https://codeberg.org/api/swagger
// https://scalar.val.run/codeberg.org/swagger.v1.json

type CodebergRepo struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	OriginalURL string `json:"original_url"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
	URL         string `json:"url"`
	Private     bool   `json:"private"`
}

func doCodebergRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse("https://codeberg.org")
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
		req.Header.Set("Content-Type", "application/json")
	}
	return glClient.Do(req)
}

func handleCodebergResponse(resp *http.Response, target any) (any, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return nil, err
		}
		return target, nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Codeberg API error %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error")
	}
}

func createCodebergRepo(repoName string, private bool) (*CodebergRepo, error) {
	bodyMap := map[string]any{
		"auto_init": false,
		"name":      repoName,
		"private":   private,
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	resp, err := doCodebergRequest("POST", "/api/v1/user/repos", nil, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	var repo CodebergRepo
	result, err := handleCodebergResponse(resp, &repo)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result.(*CodebergRepo), nil
	}
	return nil, fmt.Errorf("unexpected response")
}

func updateCodebergRepoPrivate(owner, repoName string, private bool) (*CodebergRepo, error) {
	bodyMap := map[string]any{
		"private": private,
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	path := "/api/v1/repos/" + owner + "/" + repoName
	resp, err := doCodebergRequest("PATCH", path, nil, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	var repo CodebergRepo
	result, err := handleCodebergResponse(resp, &repo)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result.(*CodebergRepo), nil
	}
	return nil, fmt.Errorf("unexpected response")
}

func getCodebergRepo(owner, repoName string) (*CodebergRepo, error) {
	path := fmt.Sprintf("/api/v1/repos/%s/%s", owner, repoName)
	resp, err := doCodebergRequest("GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var repo CodebergRepo
		if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
			return nil, err
		}
		return &repo, nil
	}
	body, _ := io.ReadAll(resp.Body)
	log.Printf("Codeberg API error %d: %s", resp.StatusCode, string(body))
	return nil, fmt.Errorf("API error")
}

func checkAndValidateCodebergRepo(owner, repoName string, private bool) error {
	repo, err := getCodebergRepo(owner, repoName)
	if err != nil {
		return err
	}
	if repo == nil {
		if _, err := createCodebergRepo(repoName, private); err != nil {
			return err
		}
		log.Printf("Created Codeberg repo %s", repoName)
		return nil
	}
	if repo.Private != private {
		if _, err := updateCodebergRepoPrivate(owner, repoName, private); err != nil {
			return err
		}
		log.Printf("Updated Codeberg repo %s privacy -> %v", repoName, private)
	} else {
		log.Printf("Codeberg repo %s exists with matching privacy %v", repoName, private)
	}
	return nil
}

func syncToCodeberg(owner, token, repoName, localPath string) error {
	cbURL := fmt.Sprintf("https://codeberg.org/%s/%s.git", owner, repoName)
	pushURL := strings.Replace(cbURL, "https://", fmt.Sprintf("https://%s:%s@", owner, token), 1)
	log.Printf("Pushing %s -> Codeberg (%s) ...", repoName, owner)
	return runCmd("git", "--git-dir", localPath, "push", "--mirror", pushURL)
}
