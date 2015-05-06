package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/flynn/flynn/controller/client"
	ct "github.com/flynn/flynn/controller/types"
	"github.com/flynn/flynn/discoverd/client"
	"github.com/flynn/flynn/pkg/cluster"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

var client *controller.Client
var repos = make(map[string]string)

func run() error {
	instances, err := discoverd.GetInstances("flynn-controller", 10*time.Second)
	if err != nil {
		log.Println("error looking up controller in service discovery:", err)
		return err
	}

	client, err = controller.NewClient("", instances[0].Meta["AUTH_KEY"])
	if err != nil {
		log.Println("error creating controller client:", err)
		return err
	}

	log.Println("parsing REPOS")
	reposEnv := os.Getenv("REPOS")
	if reposEnv != "" {
		for _, s := range strings.Split(reposEnv, " ") {
			repoApp := strings.SplitN(s, "=", 2)
			if repoApp[1] == "" {
				continue
			}
			log.Printf("mapping GitHub repo %q to Flynn app %q\n", repoApp[0], repoApp[1])
			repos[repoApp[0]] = repoApp[1]
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	http.HandleFunc("/", webhook)
	log.Printf("listening for GitHub webhooks on port %s...\n", port)
	return http.ListenAndServe(":"+port, nil)
}

type Event struct {
	Ref        string     `json:"ref"`
	Deleted    bool       `json:"deleted"`
	HeadCommit Commit     `json:"head_commit"`
	Repository Repository `json:"repository"`
}

type Commit struct {
	ID string `json:"id"`
}

type Repository struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	URL      string `json:"url"`
}

func webhook(w http.ResponseWriter, req *http.Request) {
	log.Println("handling request")
	defer req.Body.Close()

	header, ok := req.Header["X-Github-Event"]
	if !ok {
		log.Println("request missing X-Github-Event header")
		http.Error(w, "missing X-Github-Event header\n", 400)
		return
	}

	name := strings.Join(header, " ")
	switch name {
	case "ping":
		log.Println("received ping event")
		fmt.Fprintln(w, "pong")
		return
	case "push":
		log.Println("received push event")
	default:
		log.Println("received unknown event:", name)
		http.Error(w, fmt.Sprintf("Unknown X-Github-Event: %s\n", name), 400)
		return
	}

	var event Event
	if err := json.NewDecoder(req.Body).Decode(&event); err != nil {
		log.Println("error decoding JSON:", err)
		http.Error(w, "invalid JSON payload", 400)
		return
	}

	if event.Deleted {
		log.Println("skipping deleted branch:", event.Ref)
		return
	}

	app, ok := repos[event.Repository.FullName]
	if !ok {
		log.Printf("skipping unknown repo: %q\n", event.Repository.FullName)
		return
	}

	go deploy(app, event.Repository.CloneURL, path.Base(event.Ref), event.HeadCommit.ID)
}

func deploy(app, url, branch, commit string) {
	log.Printf("deploying app: %s, url: %s, branch: %s, commit: %s\n", app, url, branch, commit)

	taffyRelease, err := client.GetAppRelease("taffy")
	if err != nil {
		log.Println("error getting taffy release:", err)
		return
	}

	rwc, err := client.RunJobAttached("taffy", &ct.NewJob{
		ReleaseID:  taffyRelease.ID,
		ReleaseEnv: true,
		Cmd:        []string{app, url, branch, commit},
	})
	attachClient := cluster.NewAttachClient(rwc)
	exit, err := attachClient.Receive(os.Stdout, os.Stderr)
	if err != nil {
		log.Println("error running job:", err)
	} else if exit != 0 {
		log.Println("unexpected exit status:", exit)
	} else {
		log.Println("deploy complete")
	}
}
