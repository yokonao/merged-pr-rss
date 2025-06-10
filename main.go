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

// RepositoryStats represents statistics for a repository's RSS feed
type RepositoryStats struct {
	Repository  Repository
	PRCount     int
	Filename    string
	LastUpdated time.Time
}

func main() {
	// Load configuration
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create docs directory for GitHub Pages
	err = os.MkdirAll("docs", 0755)
	if err != nil {
		log.Fatalf("Failed to create docs directory: %v", err)
	}

	var repositoryStats []RepositoryStats

	// Process each repository separately
	for _, repo := range config.Repositories {
		prs, err := fetchMergedPRs(repo, config.GitHub.MaxPRs)
		if err != nil {
			log.Printf("Failed to fetch PRs for %s/%s: %v", repo.Owner, repo.Name, err)
			continue
		}

		var repoPRs []PRData
		for _, pr := range prs {
			repoPRs = append(repoPRs, PRData{
				Title:       pr.Title,
				URL:         pr.HTMLURL,
				MergedAt:    parseTime(*pr.MergedAt),
				Author:      pr.User.Login,
				Repository:  fmt.Sprintf("%s/%s", repo.Owner, repo.Name),
				Description: repo.Description,
			})
		}

		// Sort by merged date (descending)
		sort.Slice(repoPRs, func(i, j int) bool {
			return repoPRs[i].MergedAt.After(repoPRs[j].MergedAt)
		})

		// Generate RSS feed for this repository
		filename := fmt.Sprintf("%s-%s.xml", repo.Owner, repo.Name)
		err = generateRepositoryRSSFeed(repoPRs, repo, config.RSS, filename)
		if err != nil {
			log.Printf("Failed to generate RSS feed for %s/%s: %v", repo.Owner, repo.Name, err)
			continue
		}

		repositoryStats = append(repositoryStats, RepositoryStats{
			Repository:  repo,
			PRCount:     len(repoPRs),
			Filename:    filename,
			LastUpdated: time.Now(),
		})

		fmt.Printf("RSS feed generated for %s/%s: %d PRs\n", repo.Owner, repo.Name, len(repoPRs))
	}

	// Generate index HTML page with links to all repository feeds
	err = generateRepositoryIndexHTML(repositoryStats, config.RSS)
	if err != nil {
		log.Fatalf("Failed to generate index HTML: %v", err)
	}

	fmt.Printf("All RSS feeds generated successfully! Total repositories: %d\n", len(repositoryStats))
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

// generateRepositoryRSSFeed generates an RSS feed for a specific repository
func generateRepositoryRSSFeed(prs []PRData, repo Repository, config RSSConfig, filename string) error {
	now := time.Now()

	feed := &feeds.Feed{
		Title:       fmt.Sprintf("%s - %s/%s Merged PRs", config.Title, repo.Owner, repo.Name),
		Link:        &feeds.Link{Href: fmt.Sprintf("https://github.com/%s/%s", repo.Owner, repo.Name)},
		Description: fmt.Sprintf("Recent merged pull requests from %s/%s - %s", repo.Owner, repo.Name, repo.Description),
		Author:      &feeds.Author{Name: config.Author.Name, Email: config.Author.Email},
		Created:     now,
	}

	for _, pr := range prs {
		item := &feeds.Item{
			Title:       pr.Title,
			Link:        &feeds.Link{Href: pr.URL},
			Description: fmt.Sprintf("Merged by %s - %s", pr.Author, repo.Description),
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

	// Write RSS feed to file
	filepath := fmt.Sprintf("docs/%s", filename)
	err = os.WriteFile(filepath, []byte(rss), 0644)
	if err != nil {
		return err
	}

	return nil
}

// generateRepositoryIndexHTML generates an HTML index page with links to all repository feeds
func generateRepositoryIndexHTML(stats []RepositoryStats, config RSSConfig) error {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ja">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 1000px; margin: 0 auto; padding: 20px; }
        .header { text-align: center; margin-bottom: 30px; }
        .repository-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin: 30px 0; }
        .repository-card { background: #f8f9fa; border: 1px solid #e9ecef; border-radius: 8px; padding: 20px; }
        .repository-card h3 { margin: 0 0 10px 0; color: #333; }
        .repository-card .description { color: #666; font-size: 14px; margin: 10px 0; }
        .repository-card .stats { color: #888; font-size: 12px; margin: 10px 0; }
        .rss-link { display: inline-block; background: #ff6600; color: white; padding: 8px 16px; text-decoration: none; border-radius: 4px; font-size: 14px; }
        .rss-link:hover { background: #e55a00; }
        .footer { text-align: center; margin-top: 40px; color: #666; border-top: 1px solid #eee; padding-top: 20px; }
        .summary { background: #e3f2fd; padding: 20px; border-radius: 8px; margin: 20px 0; text-align: center; }
    </style>
</head>
<body>
    <div class="header">
        <h1>%s</h1>
        <p>%s</p>
    </div>
    
    <div class="summary">
        <h3>監視中のリポジトリ</h3>
        <p><strong>%d</strong> 個のリポジトリから最新のマージされたPRを配信しています</p>
        <p>最終更新: %s</p>
    </div>

    <div class="repository-list">`,
		config.Title,
		config.Title,
		config.Description,
		len(stats),
		time.Now().Format("2006-01-02 15:04:05 JST"))

	for _, stat := range stats {
		totalPRs := stat.PRCount
		html += fmt.Sprintf(`
        <div class="repository-card">
            <h3>%s/%s</h3>
            <div class="description">%s</div>
            <div class="stats">PRs: %d件 | 更新: %s</div>
            <a href="%s" class="rss-link">RSS Feed</a>
        </div>`,
			stat.Repository.Owner,
			stat.Repository.Name,
			stat.Repository.Description,
			totalPRs,
			stat.LastUpdated.Format("01/02 15:04"),
			stat.Filename)
	}

	html += `
    </div>
    
    <div class="footer">
        <p>各リポジトリのRSSフィードは毎日自動更新されます</p>
        <p>GitHubの最新のマージされたPull Requestを確認できます</p>
    </div>
</body>
</html>`

	err := os.WriteFile("docs/index.html", []byte(html), 0644)
	return err
}
