package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"math"
	"sort"
	"strings"
	"time"
)

func collectRepository(repos []*github.Repository) []group {
	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].GetName()) < strings.ToLower(repos[j].GetName())
	})

	groups := make([]group, 1)
	indexMap := make(map[string]int)
	index := 1
	for _, repo := range repos {
		name := repo.GetLanguage()
		if name == "" {
			name = "Others"
		}
		if indexMap[name] == 0 {
			indexMap[name] = index
			groups = append(groups, group{name: name})
			index++
		}
		groups[indexMap[name]].repo = append(groups[indexMap[name]].repo, repo)
	}
	g := groups[1:]
	sort.Slice(g, func(i, j int) bool {
		return g[i].name < g[j].name
	})
	return g
}

type repos []*github.Repository

type group struct {
	name string
	repo repos
}

func buildCollectContent(group []group) *strings.Builder {
	sb := strings.Builder{}

	for _, v := range group {
		anchor := strings.ReplaceAll(strings.ToLower(v.name), " ", "-")
		sb.WriteString(fmt.Sprintf("- [%s](#%s) (%d)\n", v.name, anchor, len(v.repo)))
	}
	sb.WriteString("\n")
	for _, v := range group {
		sb.WriteString(fmt.Sprintf("## %s\n\n", v.name))
		for _, repo := range v.repo {
			description := strings.Replace(repo.GetDescription(), "\n", " ", -1)
			sb.WriteString(fmt.Sprintf("- [%s](%s) %s %s\n", repo.GetFullName(), repo.GetHTMLURL(),
				extend(repo), description))
		}
		sb.WriteString("\n")
	}

	return &sb
}

func extend(repo *github.Repository) string {
	var time string
	if repo.PushedAt == nil {
		time = ""
	} else {
		time = repo.PushedAt.Format("2006-01")
	}
	stargazers := repo.GetStargazersCount()
	forks := repo.GetForksCount()
	return fmt.Sprintf("pushed_at:%s star:%.1fk fork:%.1fk", time, float64(stargazers)/1000, float64(forks)/1000)
}

type usertype string

const (
	user usertype = "user" // user
	org  usertype = "org"  // organization
)

type collectConfig struct {
	Name       string   `yaml:"name"`     // name
	UserType   usertype `yaml:"userType"` // user or organization
	OutputFile string   `yaml:"file"`     // file path
}

func main() {
	var (
		token        = flag.String("token", "", "github token")
		username     = flag.String("username", "", "github username")
		repository   = flag.String("repository", "", "update repository")
		file         = flag.String("file", "", "commit file name")
		branch       = flag.String("branch", "", "commit branch")
		config       = flag.String("config", "", "config")
		commitAuthor = flag.String("commitAuthor", "github-actions[bot]", "commit author")
		commitEmail  = flag.String("commitEmail", "41898282+github-actions[bot]@users.noreply.github.com", "commit email")
	)
	flag.Parse()

	if *username == "" {
		fmt.Println("no username")
		return
	}

	var configs []collectConfig

	if *config == "" {
		configs = append(configs, collectConfig{
			Name:       *username,
			UserType:   user,
			OutputFile: *file,
		})
	} else {
		configStr, err := ioutil.ReadFile(*config)
		if err != nil {
			fmt.Println(err)
			return
		}
		err = yaml.Unmarshal(configStr, &configs)
		if err != nil {
			fmt.Println(err)
			return
		}
		for i := 0; i < len(configs); {
			if configs[i].UserType != user && configs[i].UserType != org {
				configs = append(configs[:i], configs[i+1:]...)
				continue
			}
			i++
		}
	}
	var client *github.Client
	if *token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: *token},
		)
		tc := oauth2.NewClient(context.Background(), ts)
		client = github.NewClient(tc)
	} else {
		client = github.NewClient(nil)
	}

	collectRepos := make(map[string]string)
	var nofile []string
	for _, c := range configs {
		var content string
		switch c.UserType {
		case user:
			if u, err := queryList(starredByUser(client, c.Name), math.MaxInt32); err == nil {
				content = buildUserStartContent(u, *username, c)
			}
		case org:
			if u, err := queryList(reposByOrg(client, c.Name), math.MaxInt32); err == nil {
				content = buildOrgReposContent(u, *username, c)
			}
		default:
			content = ""
		}
		if c.OutputFile != "" && content != "" {
			collectRepos[c.OutputFile] = content
		} else {
			nofile = append(nofile, content)
		}
	}

	if len(collectRepos) > 0 && *username != "" && *repository != "" && *branch != "" {
		err := commit(client, *username, *repository, *branch, *commitAuthor, *commitEmail, collectRepos)
		if err != nil {
			fmt.Println("commit err", err)
		}
	} else {
		for k, v := range collectRepos {
			if err := ioutil.WriteFile(k, []byte(v), 0644); err != nil {
				fmt.Println("write file err:", err)
			}
		}
	}
	for _, v := range nofile {
		fmt.Println(v)
	}
}

func commit(client *github.Client, owner string, repo string, branch string,
	commitAuthor, commitEmail string,
	contents map[string]string) error {
	ref, _, err := client.Git.GetRef(context.Background(), owner, repo, "heads/"+branch)
	if err != nil {
		return err
	}
	var entries []*github.TreeEntry
	for file, content := range contents {
		entries = append(entries, &github.TreeEntry{Path: github.String(file),
			Type:    github.String("blob"),
			Content: github.String(content),
			Mode:    github.String("100644"),
		})
	}

	tree, _, err := client.Git.CreateTree(context.Background(), owner, repo, *ref.Object.SHA, entries)
	if err != nil {
		return err
	}

	parent, _, err := client.Repositories.GetCommit(context.Background(), owner, repo, *ref.Object.SHA)
	if err != nil {
		return err
	}
	parent.Commit.SHA = parent.SHA

	date := time.Now()
	author := &github.CommitAuthor{Date: &date, Name: &commitAuthor, Email: &commitEmail}
	commit := &github.Commit{Author: author,
		Message: &message,
		Tree:    tree,
		Parents: []*github.Commit{parent.Commit}}
	newCommit, _, err := client.Git.CreateCommit(context.Background(), owner, repo, commit)
	if err != nil {
		return err
	}
	ref.Object.SHA = newCommit.SHA
	_, _, err = client.Git.UpdateRef(context.Background(), owner, repo, ref, false)
	return err
}

func starredByUser(client *github.Client, username string) func(github.ListOptions) ([]*github.Repository, *github.Response, error) {
	return func(options github.ListOptions) ([]*github.Repository, *github.Response, error) {
		opt := github.ActivityListStarredOptions{
			ListOptions: options,
		}
		stars, resp, err := client.Activity.ListStarred(context.Background(), username, &opt)
		if err != nil {
			return nil, nil, err
		}
		r := make([]*github.Repository, len(stars))
		for i, v := range stars {
			r[i] = v.GetRepository()
		}
		return r, resp, err
	}
}

func reposByOrg(client *github.Client, org string) func(github.ListOptions) ([]*github.Repository, *github.Response, error) {
	return func(options github.ListOptions) ([]*github.Repository, *github.Response, error) {
		opt := github.RepositoryListByOrgOptions{
			ListOptions: options,
		}
		repose, resp, err := client.Repositories.ListByOrg(context.Background(), org, &opt)
		if err != nil {
			return nil, nil, err
		}
		return repose, resp, err
	}
}

func queryList(api func(listOpt github.ListOptions) ([]*github.Repository, *github.Response, error), count int) ([]*github.Repository, error) {
	var repos []*github.Repository
	subList := func(err error) ([]*github.Repository, error) {
		if count > len(repos) {
			return repos, err
		}
		return repos[0:count], err
	}
	page := 1
	for len(repos) <= count {
		opt := github.ListOptions{
			Page: page,
		}
		repo, resp, err := api(opt)
		if len(repo) > 0 {
			repos = append(repos, repo...)
		}
		if err != nil {
			return subList(err)
		}
		page = resp.NextPage
		if resp.NextPage == 0 {
			break
		}
	}
	return subList(nil)
}

func buildUserStartContent(u []*github.Repository, licenseUser string, config collectConfig) string {
	repoGroup := collectRepository(u)
	sb := buildCollectContent(repoGroup)
	return fmt.Sprintf(starsDesc, len(u)) + sb.String() + fmt.Sprintf(license, licenseUser, licenseUser)
}

func buildOrgReposContent(u []*github.Repository, licenseUser string, config collectConfig) string {
	repoGroup := collectRepository(u)
	sb := buildCollectContent(repoGroup)
	return fmt.Sprintf(reposDesc, config.Name, len(u)) + sb.String() + fmt.Sprintf(license, licenseUser, licenseUser)
}
