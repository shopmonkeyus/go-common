package gcp

import (
	"context"
	"fmt"
	"os"

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
	credentials, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return "", fmt.Errorf("error finding default google credentials: %w", err)
	}
	return credentials.ProjectID, nil
}
