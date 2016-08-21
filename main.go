package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
)

const VERSION = "v1.0.0"

var (
	interval    int64
	token       string
	v           bool
	lastChecked time.Time
)

func init() {
	flag.Int64Var(&interval, "interval", 60, "minutes to wait between checking for notifications")
	flag.StringVar(&token, "token", "", "GitHub api token")
	flag.BoolVar(&v, "v", false, "show version")

	flag.Parse()

	if v {
		fmt.Println(VERSION)
		return
	}

	if token == "" {
		fmt.Fprintf(os.Stderr, "Please provide api token\n")
		os.Exit(1)
	}
}

func main() {
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		<-exitChan
		fmt.Printf("[INFO] Received signal, exiting...\n")
		os.Exit(0)
	}()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	for {
		fmt.Printf("[INFO] Processing notifications\n")

		page := 1
		if err := processNotifications(client, page); err != nil {
			fmt.Printf("[ERROR] %s\n", err)
			os.Exit(2)
		}

		lastChecked = time.Now()
		fmt.Printf("[INFO] Sleeping for %d minutes\n", interval)
		time.Sleep(time.Duration(interval) * time.Minute)
	}
}

func processNotifications(client *github.Client, page int) error {
	listOpts := &github.NotificationListOptions{
		All:   true,
		Since: lastChecked,
		ListOptions: github.ListOptions{
			Page:    page,
			PerPage: 20,
		},
	}

	notifications, resp, err := client.Activity.ListNotifications(listOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", err)
		os.Exit(1)
	}

	for _, notification := range notifications {
		fmt.Printf("[INFO] Processing: %s\n", *notification.Subject.Title)
		if *notification.Subject.Type == "PullRequest" {
			if err := checkPullRequest(client, notification); err != nil {
				return err
			}
		}
	}

	if page == resp.LastPage || resp.NextPage == 0 {
		return nil
	}
	page = resp.NextPage
	return processNotifications(client, page)
}

func checkPullRequest(client *github.Client, notification *github.Notification) error {
	parts := strings.Split(*notification.Subject.URL, "/")
	end := parts[len(parts)-1]
	prId, err := strconv.Atoi(end)
	if err != nil {
		return err
	}

	pr, _, err := client.PullRequests.Get(
		*notification.Repository.Owner.Login,
		*notification.Repository.Name,
		int(prId),
	)
	if err != nil {
		return err
	}

	if *pr.State == "closed" && !*pr.Merged {
		fmt.Printf("[INFO] ==> Found unmerged PR, marking notification as read\n")
		resp, err := client.Activity.MarkThreadRead(*notification.ID)
		if err != nil {
			return err
		}

		fmt.Printf("[INFO] ==> Response code %d\n", resp.StatusCode)
	}

	return nil
}
