package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/go-github/v33/github"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Schema from https://shields.io/endpoint
type shieldSchema struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
	NamedLogo     string `json:"namedLogo"`
}

// create sets some default and required fields
func (s shieldSchema) create() shieldSchema {
	s.SchemaVersion = 1 // always 1
	s.NamedLogo = "GitHub Actions"
	s.Color = "brightgreen"
	return s
}

type ciProvider interface {
	getClient()
	getStatus(vars map[string]string) (shieldSchema, error)
}

var users []string
var token string
var ctx = context.Background()
var ci ciProvider

func main() {
	err := setVarsFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	ci = &GitHub{}
	ci.getClient()

	router := mux.NewRouter()
	router.HandleFunc("/health", HealthCheckHandler)
	router.HandleFunc("/ci/status/{user}/{repo}/{branch}/", CiStatusHandler)

	srv := &http.Server{
		Handler: router,
		Addr:    "localhost:80",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	fmt.Println("GO web server is running on http://" + srv.Addr)
	log.Fatal(srv.ListenAndServe())
}

// GitHub CI implementation
type GitHub struct {
	client *github.Client
}

// getGitHubClient creates GitHub API client
func (t *GitHub) getClient() {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	t.client = github.NewClient(tc)
}

// getGitHubStatus gets actual data via GitHub API
func (t *GitHub) getStatus(vars map[string]string) (shieldSchema, error) {
	s := shieldSchema{}.create()
	ListWorkflowRunsOptions := &github.ListWorkflowRunsOptions{Branch: vars["branch"], ListOptions: github.ListOptions{}}
	runs, _, err := t.client.Actions.ListRepositoryWorkflowRuns(ctx, vars["user"], vars["repo"], ListWorkflowRunsOptions)
	if err != nil {
		return s, err
	}
	if len(runs.WorkflowRuns) == 0 {
		err = errors.New("workflow not found")
		return s, err
	}

	//fmt.Println(len(runs.WorkflowRuns))
	//fmt.Println(runs.WorkflowRuns[0].Status)
	//fmt.Printf("%+v\n", *runs.WorkflowRuns[0].Status)
	//fmt.Printf("%+v\n", *runs.WorkflowRuns[0].Conclusion)
	//fmt.Printf("%+v\n", runs.WorkflowRuns[0].GetConclusion())
	//output := render.AsCode(runs)
	//fmt.Println(output)

	workflow, _, err := t.client.Actions.GetWorkflowByID(ctx, vars["user"], vars["repo"], runs.WorkflowRuns[0].GetWorkflowID())
	if err != nil {
		return s, err
	}
	//fmt.Printf("%+v\n", workflow.GetName())
	s.Label = vars["user"] + "/" + vars["repo"]
	s.Message = workflow.GetName() + " - " + runs.WorkflowRuns[0].GetConclusion()
	if runs.WorkflowRuns[0].GetConclusion() != "success" {
		s.Color = "red"
	}
	return s, nil
}

// Route handlers

// CiStatusHandler returns actual data for a badge in JSON format
func CiStatusHandler(w http.ResponseWriter, req *http.Request) {
	log.Print("hit")
	vars := mux.Vars(req)
	resp, err := ci.getStatus(vars)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(users) > 0 && !Contains(users, vars["user"]) {
		err = errors.New("user not allowed")
		log.Println(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

// HealthCheckHandler returns service health message in JSON format
func HealthCheckHandler(w http.ResponseWriter, req *http.Request) {
	json.NewEncoder(w).Encode(map[string]bool{"alive": true})
}

// Some helper functions

// setVarsFromEnv sets global vars from .env
func setVarsFromEnv() error {
	token = os.Getenv("GITHUB_ACCESS_TOKEN")
	if token == "" {
		return errors.New("No GITHUB_ACCESS_TOKEN set in .env")
	}

	au := os.Getenv("ALLOWED_USERS")
	if au != "" {
		users = strings.Split(au, ",")
	}

	return nil
}

// Contains tells whether a contains x.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}
