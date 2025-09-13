// https://github.com/settings/tokens
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type GitHubRepo struct {
	Name     string `json:"name"`
	CloneURL string `json:"clone_url"`
	Private  bool   `json:"private"`
}

func doGitHubRequest(method, path string, queryParams map[string]string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse("https://api.github.com")
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
	req.SetBasicAuth(config.GitHubUser, config.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	return ghClient.Do(req)
}

func handleGitHubResponse(resp *http.Response, target any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return err
		}
		return nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("GitHub API error %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("GitHub API error")
	}
}

// https://docs.github.com/en/rest/repos/repos#list-repositories-for-the-authenticated-user
func getGitHubRepos() ([]GitHubRepo, error) {
	var repos []GitHubRepo
	page := 1
	for {
		resp, err := doGitHubRequest("GET", "/user/repos", map[string]string{
			"per_page":    strconv.Itoa(config.PerPage),
			"page":        strconv.Itoa(page),
			"affiliation": "owner,member",
		}, nil)
		if err != nil {
			return nil, err
		}
		var batch []GitHubRepo
		if err := handleGitHubResponse(resp, &batch); err != nil {
			break
		}
		if len(batch) == 0 {
			break
		}
		repos = append(repos, batch...)
		page++
		time.Sleep(config.SleepBetweenAPI)
	}
	log.Printf("Found %d GitHub repos", len(repos))
	// List each repo with its private flag
	for _, r := range repos {
		log.Printf("- %s (private: %v)", r.Name, r.Private)
	}

	return repos, nil
}

func mirrorReposFromGitHub(repoName, githubURL, localPath string) error {
	authCloneURL := strings.Replace(githubURL, "https://", fmt.Sprintf("https://%s:%s@", config.GitHubUser, config.GitHubToken), 1)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		log.Printf("Cloning (mirror) %s ...", repoName)
		return runCmd("git", "clone", "--mirror", authCloneURL, localPath)
	} else {
		err := runCmd("git", "--git-dir", localPath, "fetch", "--all", "--prune")
		if err != nil {
			log.Printf("Recloning %s due to fetch failure", repoName)
			os.RemoveAll(localPath)
			return runCmd("git", "clone", "--mirror", authCloneURL, localPath)
		}
		return nil
	}
}
