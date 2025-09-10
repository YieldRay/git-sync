package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	GitHubUser      string
	GitHubToken     string
	GitLabUser      string
	GitLabGroup     string
	GitLabToken     string
	CodebergUser    string
	CodebergToken   string
	RepoVisibility  string
	PerPage         int
	BackupDir       string
	LogsFolder      string
	SleepBetweenAPI time.Duration
}

var config Config
var ghClient *http.Client
var glClient *http.Client

func loadConfig(target string) Config {
	cfg := Config{
		GitHubUser:      mustGetEnv("GITHUB_USER"),
		GitHubToken:     mustGetEnv("GITHUB_TOKEN"),
		RepoVisibility:  getEnv("REPO_VISIBILITY", "auto"),
		PerPage:         100,
		BackupDir:       "./repos-backup",
		LogsFolder:      "./log",
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

func initClients() {
	ghClient = &http.Client{}
	glClient = &http.Client{}
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

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	target := flag.String("target", "gitlab", "sync target: gitlab or codeberg")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -target {gitlab|codeberg}\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Targets and required environment:")
		fmt.Fprintln(os.Stderr, "  gitlab   -> requires GITLAB_USER, GITLAB_TOKEN; optional GITLAB_GROUP")
		fmt.Fprintln(os.Stderr, "  codeberg -> requires CODEBERG_USER, CODEBERG_TOKEN")
		fmt.Fprintln(os.Stderr, "Always required:")
		fmt.Fprintln(os.Stderr, "  GITHUB_USER, GITHUB_TOKEN")
		fmt.Fprintln(os.Stderr, "Optional:")
		fmt.Fprintln(os.Stderr, "  REPO_VISIBILITY (auto|public|private), default=auto")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *target != "gitlab" && *target != "codeberg" {
		fmt.Fprintf(os.Stderr, "Invalid -target: %q\n\n", *target)
		flag.Usage()
		os.Exit(2)
	}

	config = loadConfig(*target)
	setupLogger()
	initClients()
	log.Printf("####################### logger Started ############################")
	log.Printf("#################### Timestamp: %s ######################", time.Now().Format("2006-01-02 15:04:05"))

	repos, err := getGitHubRepos()
	if err != nil {
		log.Fatal(err)
	}
	var gitlabGroupID *int
	if *target == "gitlab" {
		gitlabGroupID, err = getGitLabGroupID()
		if err != nil {
			log.Fatal(err)
		}
	}
	if len(repos) == 0 {
		log.Printf("No repos found; exiting.")
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

		log.Printf("Syncing %s", repoName)
		err := mirrorReposFromGitHub(repoName, githubURL, localPath)
		if err != nil {
			log.Printf("Failed to mirror %s: %v", repoName, err)
			continue
		}
		switch *target {
		case "gitlab":
			err = checkAndValidateGitLabRepos(gitlabGroupID, repoName, config.GitLabUser, repoVisibility)
			if err != nil {
				log.Printf("Failed to validate GitLab repo %s: %v", repoName, err)
				continue
			}
			err = syncRepos(gitlabGroupID, config.GitLabUser, config.GitLabToken, repoName, localPath)
			if err != nil {
				log.Printf("Failed to sync %s: %v", repoName, err)
				continue
			}
		case "codeberg":
			if config.CodebergUser == "" || config.CodebergToken == "" {
				log.Fatalf("CODEBERG_USER and CODEBERG_TOKEN must be set when target=codeberg")
			}
			private := repoVisibility == "private"
			if err := checkAndValidateCodebergRepo(config.CodebergUser, repoName, private); err != nil {
				log.Printf("Failed to validate Codeberg repo %s: %v", repoName, err)
				continue
			}
			if err := syncToCodeberg(config.CodebergUser, config.CodebergToken, repoName, localPath); err != nil {
				log.Printf("Failed to sync to Codeberg %s: %v", repoName, err)
				continue
			}
		default:
			log.Fatalf("Unknown target: %s (expected gitlab or codeberg)", *target)
		}
		log.Printf("✅ Synced %s", repoName)
		reposDone++
		log.Printf("Repos done: %d/%d", reposDone, len(repos))
	}
	log.Printf("✅ All Done :), all repositories has been synced, please check the logs for details.")
	log.Printf("✅ All Done :), Now you can enjoy hehe, please check the logs for details.")
}
