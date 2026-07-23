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
	detail  string
}

func (e *ProblemError) Error() string {
	if e.detail != "" {
		return e.detail
	}
	if e.Problem.Detail != "" {
		return e.Problem.Detail
	}
	return e.Problem.Title
}

// NewProblemError constructs a typed problem safe to expose to clients.
func NewProblemError(status int, code, title, detail string) *ProblemError {
	return &ProblemError{
		Problem: Problem{
			Type:   problemBaseURL + strings.ReplaceAll(code, "_", "-"),
			Title:  title,
			Status: status,
			Detail: safeProblemDetail(code),
			Code:   code,
		},
		detail: detail,
	}
}

func problemFromError(err error) Problem {
	var problemErr *ProblemError
	if errors.As(err, &problemErr) {
		problem := problemErr.Problem
		problem.Detail = safeProblemDetail(problem.Code)
		return problem
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
	problem.Detail = safeProblemDetail(problem.Code)
	problem.RequestID = RequestID(r.Context())
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func safeProblemDetail(code string) string {
	switch code {
	case "unauthorized":
		return "A valid bearer token is required."
	case "origin_forbidden":
		return "The request origin is not allowed."
	case "invalid_request":
		return "The request is invalid."
	case "request_too_large":
		return "The request exceeds the server size limit."
	case "request_timeout":
		return "The operation was cancelled or exceeded its deadline."
	case "input_too_large":
		return "The upload exceeds the effective size limit."
	case "invalid_upload":
		return "The request does not describe a valid Airplan upload."
	case "invalid_target":
		return "The target is not a valid marker-managed Airplan upload."
	case "upload_not_found":
		return "The marker-managed upload could not be found."
	default:
		return "The server could not complete the request."
	}
}
