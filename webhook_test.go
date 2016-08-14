package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/flynn/flynn/pkg/postgres"
	"github.com/jackc/pgx"
)

func setupTestDB(dbname string) (*postgres.DB, error) {
	conf := pgx.ConnConfig{
		Host:     os.Getenv("PGHOST"),
		Database: "postgres",
	}
	db, err := pgx.Connect(conf)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbname)); err != nil {
		return nil, err
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbname)); err != nil {
		return nil, err
	}
	pool, err := pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig: pgx.ConnConfig{
			Host:     os.Getenv("PGHOST"),
			Database: dbname,
		},
	})
	if err != nil {
		return nil, err
	}
	d := postgres.New(pool, nil)
	return d, setupDB(d)
}

// TestCreateRepo tests that repos can be created via the HTTP API
func TestCreateRepo(t *testing.T) {
	db, err := setupTestDB("flynn_webhook_test")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := httptest.NewServer(NewServer(db, nil, nil))
	defer s.Close()

	data := bytes.NewBuffer([]byte("name=lmars/foo&branch=master&app=foo"))
	res, err := http.Post(s.URL+"/repos", "application/x-www-form-urlencoded", data)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected ok response, got %s", res.Status)
	}

	res, err = http.Get(s.URL + "/repos.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected ok response, got %s", res.Status)
	}
	var repos []Repo
	if err := json.NewDecoder(res.Body).Decode(&repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	repo := repos[0]
	if repo.Name != "lmars/foo" {
		t.Fatalf(`expected repo name "lmars/foo", got %q`, repo.Name)
	}
	if repo.Branch != "master" {
		t.Fatalf(`expected repo branch "master", got %q`, repo.Branch)
	}
	if repo.App != "foo" {
		t.Fatalf(`expected repo app "master", got %q`, repo.App)
	}
}
