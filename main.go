package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	GitHubUser      string
	GitHubToken     string
	GitLabUser      string
	GitLabGroup     string
	GitLabToken     string
	CodebergUser    string
	CodebergToken   string
	BitbucketEmail  string
	BitbucketToken  string
	BitbucketWs     string
	RepoVisibility  string
	PerPage         int
	BackupDir       string
	LogsFolder      string
	SleepBetweenAPI time.Duration
}

var config Config
var ghClient = &http.Client{Transport: transport}
var glClient = &http.Client{Transport: transport}
var bbClient = &http.Client{Transport: transport}
var cbClient = &http.Client{Transport: transport}

func loadConfig(target string) Config {
	cfg := Config{
		GitHubUser:      mustGetEnv("GITHUB_USER"),
		GitHubToken:     mustGetEnv("GITHUB_TOKEN"),
		RepoVisibility:  getEnv("REPO_VISIBILITY", "auto"),
		PerPage:         100,
		BackupDir:       "./repos-backup",
		LogsFolder:      "./logs",
		SleepBetweenAPI: 500 * time.Millisecond,
	}
	switch target {
	case "gitlab":
		cfg.GitLabUser = mustGetEnv("GITLAB_USER")
		cfg.GitLabToken = mustGetEnv("GITLAB_TOKEN")
		cfg.GitLabGroup = getEnv("GITLAB_GROUP", "")
	case "codeberg":
		cfg.CodebergUser = mustGetEnv("CODEBERG_USER")
		cfg.CodebergToken = mustGetEnv("CODEBERG_TOKEN")
	case "bitbucket":
		cfg.BitbucketEmail = mustGetEnv("BITBUCKET_EMAIL")
		cfg.BitbucketToken = mustGetEnv("BITBUCKET_TOKEN")
		// Workspace is required for Bitbucket API
		cfg.BitbucketWs = mustGetEnv("BITBUCKET_WORKSPACE")
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func mustGetEnv(key string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	log.Fatalf("Environment variable %s is not set.", key)
	return ""
}

func setupLogger() {
	os.MkdirAll(config.LogsFolder, 0755)
	timestamp := time.Now().Format("20060102_150405")
	logFilePath := filepath.Join(config.LogsFolder, fmt.Sprintf("logs_%s.txt", timestamp))
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags)
}

func main() {
	target := flag.String("target", "", "sync target: gitlab | codeberg | bitbucket")
	repoFilter := flag.String("repo", "", "if set, only sync this specific GitHub repo (test mode)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage: %s -target {gitlab|codeberg|bitbucket}\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Targets and required environment:")
		fmt.Fprintln(os.Stderr, "  gitlab   -> requires GITLAB_USER, GITLAB_TOKEN; optional GITLAB_GROUP")
		fmt.Fprintln(os.Stderr, "  codeberg -> requires CODEBERG_USER, CODEBERG_TOKEN")
		fmt.Fprintln(os.Stderr, "  bitbucket-> requires BITBUCKET_EMAIL, BITBUCKET_TOKEN, BITBUCKET_WORKSPACE")
		fmt.Fprintln(os.Stderr, "Always required:")
		fmt.Fprintln(os.Stderr, "  GITHUB_USER, GITHUB_TOKEN")
		fmt.Fprintln(os.Stderr, "Optional:")
		fmt.Fprintln(os.Stderr, "  REPO_VISIBILITY (auto|public|private), default=auto")

	}
	flag.Parse()
	if *target != "gitlab" && *target != "codeberg" && *target != "bitbucket" {
		fmt.Fprintf(os.Stderr, "Invalid -target: %q\n\n", *target)
		flag.Usage()
		os.Exit(2)
	}

	config = loadConfig(*target)
	// before this line, the logger will print to stdout
	setupLogger()
	// after this line, all logs will go to the log file
	log.Printf("🔔 Logger started")
	log.Printf("🕒 Timestamp: %s", time.Now().Format("2006-01-02 15:04:05"))

	repos, err := getGitHubRepos()
	if err != nil {
		log.Fatal(err)
	}
	// Test mode: filter to a specific repo if flag provided
	if *repoFilter != "" {
		log.Printf("Test mode: filtering to repository %s", *repoFilter)
		var filtered []GitHubRepo
		for _, r := range repos {
			if r.Name == *repoFilter {
				filtered = append(filtered, r)
				break
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("Test mode: repository %s not found among GitHub repos", *repoFilter)
		}
		repos = filtered

		// Print list of repos to sync using Map from utils.go
		log.Printf("📦 Will sync the following repositories: %s", strings.Join(
			Map(repos, func(r GitHubRepo) string { return r.Name }), ", "))

	}
	var gitlabGroupID *int
	if *target == "gitlab" {
		gitlabGroupID, err = getGitLabGroupID()
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(repos) == 0 {
		log.Printf("🚫 No repos found; exiting.")
		return
	}

	os.MkdirAll(config.BackupDir, 0755)
	reposDone := 0
	for _, repo := range repos {
		repoName := repo.Name
		githubURL := repo.CloneURL
		repoVisibility := "private"
		if config.RepoVisibility == "auto" {
			if repo.Private {
				repoVisibility = "private"
			} else {
				repoVisibility = "public"
			}
		} else if config.RepoVisibility != "" {
			repoVisibility = config.RepoVisibility
		}
		localPath := filepath.Join(config.BackupDir, fmt.Sprintf("%s.git", repoName))

		log.Printf("🌐 Syncing %s", repoName)
		err := mirrorReposFromGitHub(repoName, githubURL, localPath)
		if err != nil {
			log.Printf("🚫 Failed to mirror %s: %v", repoName, err)
			continue
		}
		switch *target {
		case "gitlab":
			err = checkAndValidateGitLabRepos(gitlabGroupID, repoName, config.GitLabUser, repoVisibility)
			if err != nil {
				log.Printf("🚫 Failed to validate GitLab repo %s: %v", repoName, err)
				continue
			}
			err = syncRepos(gitlabGroupID, config.GitLabUser, config.GitLabToken, repoName, localPath)
			if err != nil {
				log.Printf("🚫 Failed to sync %s: %v", repoName, err)
				continue
			}
		case "codeberg":
			if config.CodebergUser == "" || config.CodebergToken == "" {
				log.Fatalf("🚫 CODEBERG_USER and CODEBERG_TOKEN must be set when target=codeberg")
			}
			private := repoVisibility == "private"
			if err := checkAndValidateCodebergRepo(config.CodebergUser, repoName, private); err != nil {
				log.Printf("🚫 Failed to validate Codeberg repo %s: %v", repoName, err)
				continue
			}
			if err := syncToCodeberg(config.CodebergUser, config.CodebergToken, repoName, localPath); err != nil {
				log.Printf("🚫 Failed to sync to Codeberg %s: %v", repoName, err)
				continue
			}
		case "bitbucket":
			if config.BitbucketEmail == "" || config.BitbucketToken == "" || config.BitbucketWs == "" {
				log.Fatalf("🚫 BITBUCKET_EMAIL, BITBUCKET_TOKEN, and BITBUCKET_WORKSPACE must be set when target=bitbucket")
			}
			private := repoVisibility == "private"
			if err := checkAndValidateBitbucketRepo(config.BitbucketWs, repoName, private); err != nil {
				log.Printf("🚫 Failed to validate Bitbucket repo %s: %v", repoName, err)
				continue
			}
			if err := syncToBitbucket(config.BitbucketEmail, config.BitbucketToken, config.BitbucketWs, repoName, localPath); err != nil {
				log.Printf("🚫 Failed to sync to Bitbucket %s: %v", repoName, err)
				continue
			}
		default:
			log.Fatalf("🚫 Unknown target: %s (expected gitlab or codeberg)", *target)
		}
		log.Printf("✅ Synced %s", repoName)
		reposDone++
		log.Printf("Repos done: %d/%d", reposDone, len(repos))
	}
	log.Printf("✅ All Done :), all repositories has been synced, please check the logs for details.")
}
