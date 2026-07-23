package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const problemBaseURL = "https://airplan.dev/problems/"

// ProblemError lets the operation adapter return a safe, typed HTTP failure.
type ProblemError struct {
	Problem Problem
}

func (e *ProblemError) Error() string {
	if e.Problem.Detail != "" {
		return e.Problem.Detail
	}
	return e.Problem.Title
}

// NewProblemError constructs a typed problem safe to expose to clients.
func NewProblemError(status int, code, title, detail string) *ProblemError {
	return &ProblemError{Problem: Problem{
		Type:   problemBaseURL + strings.ReplaceAll(code, "_", "-"),
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
	}}
}

func problemFromError(err error) Problem {
	var problemErr *ProblemError
	if errors.As(err, &problemErr) {
		return problemErr.Problem
	}
	return Problem{
		Type:   problemBaseURL + "internal-server-error",
		Title:  "Internal server error",
		Status: http.StatusInternalServerError,
		Detail: "The server could not complete the request.",
		Code:   "internal_server_error",
	}
}

func requestProblem(status int, code, title, detail string) Problem {
	return NewProblemError(status, code, title, detail).Problem
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, problemFromError(err))
}

func writeProblem(w http.ResponseWriter, r *http.Request, problem Problem) {
	if problem.Status < 400 || problem.Status > 599 {
		problem = problemFromError(fmt.Errorf("invalid problem status"))
	}
	problem.RequestID = RequestID(r.Context())
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
