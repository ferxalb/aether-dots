package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/github"
	"aether-dots/internal/model"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func newGitHubService() *github.Service {
	return github.NewService()
}

func githubCommand(db *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aether github [login|status|refresh|repositories|select|grant|write-grant|proposals|revoke|disconnect]")
	}
	switch args[0] {
	case "login":
		return githubLogin(db)
	case "status":
		return githubStatus(db)
	case "refresh":
		return refreshGitHubRepositories(db)
	case "repositories":
		return listGitHubRepositories(db)
	case "select":
		return selectGitHubRepositories(db, args[1:])
	case "grant":
		return grantGitHubRepositories(db, args[1:])
	case "write-grant":
		return grantGitHubWrites(db, args[1:])
	case "proposals":
		return githubProposalsCommand(db, newGitHubService(), os.Stdin, os.Stdout, stdinIsTerminal(), args[1:])
	case "revoke":
		if len(args) != 2 {
			return errors.New("usage: aether github revoke <dot-id>")
		}
		return db.SetConversationGitHubGrants(args[1], nil, false)
	case "disconnect":
		return disconnectGitHub(db)
	default:
		return fmt.Errorf("unknown GitHub command %q", args[0])
	}
}

func githubLogin(db *store.Store) error {
	service := newGitHubService()
	if !service.Installed() {
		return github.ErrCLINotInstalled
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("Using your GitHub CLI login (gh). AETHER stores no GitHub token.")
	user, err := service.Connect(ctx)
	if err != nil {
		return err
	}
	if err := db.SetGitHubConnected(true); err != nil {
		return err
	}
	if err := refreshGitHubRepositoriesWithService(db, service); err != nil {
		return err
	}
	fmt.Printf("Connected as @%s. Select repositories with `aether github select <repo-id>...`.\n", user.Login)
	return nil
}

func githubStatus(db *store.Store) error {
	snapshot := db.Snapshot()
	if !snapshot.GitHubConnected {
		fmt.Println("GitHub is disconnected in AETHER.")
		return nil
	}
	service := newGitHubService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := service.GetAuthenticatedUser(ctx)
	if err != nil {
		return err
	}
	if account := snapshot.GitHubAccount; account != "" && !strings.EqualFold(account, user.Login) {
		return fmt.Errorf("GitHub was connected as @%s but the GitHub CLI is signed in as @%s: %w", account, user.Login, github.ErrAccountChanged)
	}
	selected := 0
	for _, repository := range snapshot.GitHubRepositories {
		if repository.Selected {
			selected++
		}
	}
	fmt.Printf("Connected as @%s · %d accessible repositories · %d selected · writes only through approved proposals\n", user.Login, len(snapshot.GitHubRepositories), selected)
	if !snapshot.GitHubLastVerifiedAt.IsZero() {
		fmt.Printf("Last verified: %s\n", snapshot.GitHubLastVerifiedAt.Local().Format(time.RFC3339))
	}
	return nil
}

func refreshGitHubRepositories(db *store.Store) error {
	if !db.Snapshot().GitHubConnected {
		return github.ErrNotConnected
	}
	return refreshGitHubRepositoriesWithService(db, newGitHubService())
}

func refreshGitHubRepositoriesWithService(db *store.Store, service *github.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	discovery, err := service.Discover(ctx)
	if err != nil {
		return err
	}
	repositories := make([]task.GitHubRepository, 0, len(discovery.Repositories))
	for _, repository := range discovery.Repositories {
		repositories = append(repositories, task.GitHubRepository{
			ID:    repository.ID,
			Owner: repository.Owner.Login, Name: repository.Name, FullName: repository.FullName,
			Private: repository.Private, HTMLURL: repository.HTMLURL, DefaultBranch: repository.DefaultBranch,
		})
	}
	if err := db.SetGitHubAccount(discovery.User.Login); err != nil {
		return err
	}
	if err := db.ReplaceGitHubRepositories(repositories); err != nil {
		return err
	}
	if len(repositories) == 0 {
		return errors.New("no repositories are accessible to this GitHub CLI account; check `gh auth status`")
	}
	fmt.Printf("Refreshed %d accessible repositories.\n", len(repositories))
	return nil
}

func listGitHubRepositories(db *store.Store) error {
	if !db.Snapshot().GitHubConnected {
		return github.ErrNotConnected
	}
	for _, repository := range db.GitHubRepositories() {
		state := "not selected"
		if repository.Selected {
			state = "selected"
		}
		privacy := "public"
		if repository.Private {
			privacy = "private"
		}
		fmt.Printf("%d\t%-12s\t%-8s\t%s\t%s\n", repository.ID, state, privacy, repository.FullName, repository.HTMLURL)
	}
	return nil
}

func selectGitHubRepositories(db *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aether github select <repo-id>...")
	}
	ids, err := parseGitHubRepositoryIDs(args)
	if err != nil {
		return err
	}
	return db.SetGitHubRepositorySelection(ids)
}

func grantGitHubRepositories(db *store.Store, args []string) error {
	fs := flag.NewFlagSet("github grant", flag.ContinueOnError)
	allowModelData := fs.Bool("allow-model-data", false, "acknowledge that selected GitHub data can be sent to this Dot's configured model provider")
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) < 2 {
		return errors.New("usage: aether github grant --allow-model-data <dot-id> <repo-id>...")
	}
	if !*allowModelData {
		return errors.New("add --allow-model-data after reviewing the repository-to-model privacy disclosure")
	}
	ids, err := parseGitHubRepositoryIDs(remaining[1:])
	if err != nil {
		return err
	}
	return db.SetConversationGitHubGrants(remaining[0], ids, true)
}

func parseGitHubRepositoryIDs(args []string) ([]int64, error) {
	ids := make([]int64, 0, len(args))
	for _, raw := range args {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid GitHub repository ID %q", raw)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func disconnectGitHub(db *store.Store) error {
	localErr := db.DisconnectGitHub()
	if err := localErr; err != nil {
		return err
	}
	fmt.Println("GitHub was disconnected in AETHER: repository selections and Dot grants were removed. Your GitHub CLI login is untouched (`gh auth logout` signs it out).")
	return nil
}

func cliGitHubGenerator(db *store.Store, dotID string, service *github.Service) model.Provider {
	providerForDot := func(id string) model.Provider {
		return engine.WithGitHubTools(db, id, model.NewProvider(model.FromEnvironment()), service)
	}
	return engine.WithSpecialistDelegationFactory(db, dotID, providerForDot(dotID), providerForDot, nil)
}
