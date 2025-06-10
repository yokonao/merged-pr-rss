package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/gorilla/feeds"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration file structure
type Config struct {
	Repositories []Repository `yaml:"repositories"`
	RSS          RSSConfig    `yaml:"rss"`
	GitHub       GitHubConfig `yaml:"github"`
}

// Repository represents a GitHub repository to monitor
type Repository struct {
	Owner       string `yaml:"owner"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// RSSConfig represents RSS feed configuration
type RSSConfig struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	Link        string       `yaml:"link"`
	Author      AuthorConfig `yaml:"author"`
}

// AuthorConfig represents RSS author configuration
type AuthorConfig struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// GitHubConfig represents GitHub API configuration
type GitHubConfig struct {
	MaxPRs int `yaml:"max_prs"`
}

// PullRequest represents a GitHub pull request
type PullRequest struct {
	Number   int     `json:"number"`
	Title    string  `json:"title"`
	HTMLURL  string  `json:"html_url"`
	MergedAt *string `json:"merged_at"`
	User     User    `json:"user"`
	Base     Base    `json:"base"`
}

// User represents a GitHub user
type User struct {
	Login string `json:"login"`
}

// Base represents the base branch information
type Base struct {
	Repo GitHubRepository `json:"repo"`
}

// GitHubRepository represents a GitHub repository in API response
type GitHubRepository struct {
	Owner GitHubUser `json:"owner"`
	Name  string     `json:"name"`
}

// GitHubUser represents a GitHub user in API response
type GitHubUser struct {
	Login string `json:"login"`
}

// PRData represents processed pull request data
type PRData struct {
	Title       string
	URL         string
	MergedAt    time.Time
	Author      string
	Repository  string
	Description string
}

func main() {
	// Load configuration
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Fetch PRs from all repositories
	var allPRs []PRData
	for _, repo := range config.Repositories {
		prs, err := fetchMergedPRs(repo, config.GitHub.MaxPRs)
		if err != nil {
			log.Printf("Failed to fetch PRs for %s/%s: %v", repo.Owner, repo.Name, err)
			continue
		}

		for _, pr := range prs {
			allPRs = append(allPRs, PRData{
				Title:       pr.Title,
				URL:         pr.HTMLURL,
				MergedAt:    parseTime(*pr.MergedAt),
				Author:      pr.User.Login,
				Repository:  fmt.Sprintf("%s/%s", repo.Owner, repo.Name),
				Description: repo.Description,
			})
		}
	}

	// Sort by merged date (descending)
	sort.Slice(allPRs, func(i, j int) bool {
		return allPRs[i].MergedAt.After(allPRs[j].MergedAt)
	})

	// Generate RSS feed
	err = generateRSSFeed(allPRs, config.RSS)
	if err != nil {
		log.Fatalf("Failed to generate RSS feed: %v", err)
	}

	fmt.Println("RSS feed generated successfully!")
}

// loadConfig loads the configuration from a YAML file
func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// fetchMergedPRs fetches merged pull requests from GitHub API
func fetchMergedPRs(repo Repository, maxPRs int) ([]PullRequest, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=closed&sort=updated&direction=desc&per_page=%d",
		repo.Owner, repo.Name, maxPRs)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub token if available
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API request failed: %d %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	err = json.Unmarshal(body, &prs)
	if err != nil {
		return nil, err
	}

	// Filter only merged PRs
	var mergedPRs []PullRequest
	for _, pr := range prs {
		if pr.MergedAt != nil {
			mergedPRs = append(mergedPRs, pr)
		}
	}

	return mergedPRs, nil
}

// parseTime parses GitHub's timestamp format
func parseTime(timestamp string) time.Time {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		log.Printf("Failed to parse timestamp %s: %v", timestamp, err)
		return time.Now()
	}
	return t
}

// generateRSSFeed generates an RSS feed from pull request data
func generateRSSFeed(prs []PRData, config RSSConfig) error {
	now := time.Now()

	feed := &feeds.Feed{
		Title:       config.Title,
		Link:        &feeds.Link{Href: config.Link},
		Description: config.Description,
		Author:      &feeds.Author{Name: config.Author.Name, Email: config.Author.Email},
		Created:     now,
	}

	for _, pr := range prs {
		item := &feeds.Item{
			Title:       fmt.Sprintf("[%s] %s", pr.Repository, pr.Title),
			Link:        &feeds.Link{Href: pr.URL},
			Description: fmt.Sprintf("Merged by %s in %s - %s", pr.Author, pr.Repository, pr.Description),
			Author:      &feeds.Author{Name: pr.Author},
			Created:     pr.MergedAt,
		}
		feed.Items = append(feed.Items, item)
	}

	// Generate RSS XML
	rss, err := feed.ToRss()
	if err != nil {
		return err
	}

	// Create docs directory for GitHub Pages
	err = os.MkdirAll("docs", 0755)
	if err != nil {
		return err
	}

	// Write RSS feed to file
	err = os.WriteFile("docs/feed.xml", []byte(rss), 0644)
	if err != nil {
		return err
	}

	// Generate simple HTML index
	html := generateIndexHTML(config, len(prs))
	err = os.WriteFile("docs/index.html", []byte(html), 0644)
	if err != nil {
		return err
	}

	return nil
}

// generateIndexHTML generates a simple HTML index page
func generateIndexHTML(config RSSConfig, prCount int) string {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .header { text-align: center; margin-bottom: 30px; }
        .rss-link { display: inline-block; background: #ff6600; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; }
        .stats { background: #f5f5f5; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .footer { text-align: center; margin-top: 30px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h1>%s</h1>
        <p>%s</p>
        <a href="feed.xml" class="rss-link">RSS Feed</a>
    </div>
    
    <div class="stats">
        <h3>統計情報</h3>
        <p>現在 <strong>%d</strong> 件のマージされたPRが登録されています</p>
        <p>最終更新: %s</p>
    </div>
    
    <div class="footer">
        <p>このフィードは毎日自動更新されます</p>
    </div>
</body>
</html>`,
		config.Title,
		config.Title,
		config.Description,
		prCount,
		time.Now().Format("2006-01-02 15:04:05 JST"))

	return html
}
