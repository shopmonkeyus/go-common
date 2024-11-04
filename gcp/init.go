package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
)

// FetchProjectID will return the GCP project id
func FetchProjectID() (string, error) {
	id := os.Getenv("PROJECT_ID")
	if id != "" {
		return id, nil
	}
	ctx := context.Background()
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope, compute.DevstorageReadWriteScope)
	if err != nil {
		return "", fmt.Errorf("error finding default google credentials: %w", err)
	}
	if credentials.ProjectID != "" {
		return credentials.ProjectID, nil
	}
	if credentials.JSON != nil {
		var creds map[string]interface{}
		if err := json.Unmarshal(credentials.JSON, &creds); err != nil {
			return "", fmt.Errorf("error unmarshalling google credentials: %w", err)
		}
		if val, ok := creds["quota_project_id"].(string); ok {
			return val, nil
		}
	}
	return "", nil
}

type cleanupFunc func()

// FetchProjectIDUsingCmd will return the GCP project id using the provided cobra command
// to find the project id. It will also return a cleanup function that should be called
// to clean up any temporary files created.
func FetchProjectIDUsingCmd(cmd *cobra.Command) (string, cleanupFunc, error) {
	var cleanup cleanupFunc = func() {}
	credsVal := os.Getenv("SM_GOOGLE_CREDENTIALS")
	if credsVal != "" {
		fn, err := os.CreateTemp("", "")
		if err != nil {
			return "", cleanup, fmt.Errorf("error creating temp file: %w", err)
		}
		io.WriteString(fn, credsVal)
		fn.Close()
		cleanup = func() {
			os.Remove(fn.Name())
		}
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", fn.Name())
	}

	val, _ := cmd.Flags().GetString("project-id")
	if val != "" {
		return val, cleanup, nil
	}
	projectId := os.Getenv("GCP_PROJECT_ID")
	if projectId != "" {
		return projectId, cleanup, nil
	}
	projectId = os.Getenv("SM_GS_PROJECT_ID")
	if projectId != "" {
		return projectId, cleanup, nil
	}
	val, err := FetchProjectID()
	if err != nil {
		return "", cleanup, err
	}
	if val == "" {
		return "", cleanup, fmt.Errorf("google project id not found")
	}
	return val, cleanup, nil
}
