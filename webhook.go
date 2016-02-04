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

	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/controller/client"
	ct "github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/controller/types"
	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/discoverd/client"
	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/pkg/cluster"
	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/pkg/postgres"
	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/julienschmidt/httprouter"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	db := postgres.Wait(nil, nil)
	if err := setupDB(db); err != nil {
		return err
	}

	client, err := newControllerClient()
	if err != nil {
		return err
	}

	server := NewServer(db, client)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	log.Printf("listening for GitHub webhooks on port %s...\n", port)
	return http.ListenAndServe(":"+port, server)
}

func setupDB(db *postgres.DB) error {
	m := postgres.NewMigrations()
	m.Add(1,
		`CREATE TABLE repos (
	id serial PRIMARY KEY,
	name text NOT NULL,
	branch text NOT NULL DEFAULT 'master',
	app text NOT NULL,
	created_at timestamp with time zone NOT NULL DEFAULT current_timestamp,
	UNIQUE (name, branch)
	);`)
	return m.Migrate(db)
}

func newControllerClient() (*controller.Client, error) {
	instances, err := discoverd.GetInstances("controller", 10*time.Second)
	if err != nil {
		log.Println("error looking up controller in service discovery:", err)
		return nil, err
	}
	return controller.NewClient("", instances[0].Meta["AUTH_KEY"])
}

func NewServer(db *postgres.DB, client *controller.Client) *Server {
	s := &Server{db: db, client: client}
	s.router = httprouter.New()
	s.router.POST("/", s.webhook)
	s.router.GET("/", s.index)
	s.router.GET("/repos.json", s.getRepos)
	s.router.POST("/repos", s.createRepo)
	s.router.GET("/apps.json", s.getApps)
	s.router.ServeFiles("/assets/*filepath", http.Dir("assets"))
	return s

}

type Server struct {
	db     *postgres.DB
	client *controller.Client
	router *httprouter.Router
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.router.ServeHTTP(w, req)
}

func (s *Server) index(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	http.ServeFile(w, req, "assets/index.html")
}

type Repo struct {
	ID        int32      `json:"id"`
	Name      string     `json:"name"`
	Branch    string     `json:"branch"`
	App       string     `json:"app"`
	CreatedAt *time.Time `json:"created_at"`
}

func scanRepo(s postgres.Scanner) (Repo, error) {
	var r Repo
	return r, s.Scan(&r.ID, &r.Name, &r.Branch, &r.App, &r.CreatedAt)
}

func (s *Server) getRepos(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	rows, err := s.db.Query("SELECT id, name, branch, app, created_at FROM repos")
	if err != nil {
		log.Println("error getting repos from db:", err)
		http.Error(w, "error getting repos", 500)
		return
	}
	var repos []Repo
	for rows.Next() {
		repo, err := scanRepo(rows)
		if err != nil {
			rows.Close()
			log.Println("error scanning db row:", err)
			http.Error(w, "error getting repos", 500)
			return
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		log.Println("error scanning db rows:", err)
		http.Error(w, "error getting repos", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

func (s *Server) createRepo(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	r := Repo{
		Name:   req.FormValue("name"),
		Branch: req.FormValue("branch"),
		App:    req.FormValue("app"),
	}
	if r.Name == "" || r.App == "" {
		http.Error(w, "both name and app are required", 400)
		return
	}
	if r.Branch == "" {
		r.Branch = "master"
	}
	err := s.db.QueryRow("INSERT INTO repos (name, branch, app) VALUES ($1, $2, $3) RETURNING created_at", r.Name, r.Branch, r.App).Scan(&r.CreatedAt)
	if err != nil {
		log.Println("error adding repo to db:", err)
		http.Error(w, "error adding repo", 500)
		return
	}
	http.Redirect(w, req, "/", 302)
}

func (s *Server) getApps(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	apps, err := s.client.AppList()
	if err != nil {
		log.Println("error getting apps:", err)
		http.Error(w, "error getting apps", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

func (s *Server) getRepo(name, branch string) (Repo, error) {
	row := s.db.QueryRow("SELECT id, name, branch, app, created_at FROM repos WHERE name = $1 AND branch = $2", name, branch)
	return scanRepo(row)
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

func (s *Server) webhook(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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

	branch := path.Base(event.Ref)
	repo, err := s.getRepo(event.Repository.FullName, branch)
	if err != nil {
		log.Printf("error loading repo %q (%q branch): %s\n", event.Repository.FullName, branch, err)
		return
	}

	go s.deploy(repo.App, event.Repository.CloneURL, branch, event.HeadCommit.ID)
}

func (s *Server) deploy(app, url, branch, commit string) {
	log.Printf("deploying app: %s, url: %s, branch: %s, commit: %s\n", app, url, branch, commit)

	taffyRelease, err := s.client.GetAppRelease("taffy")
	if err != nil {
		log.Println("error getting taffy release:", err)
		return
	}

	rwc, err := s.client.RunJobAttached("taffy", &ct.NewJob{
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
