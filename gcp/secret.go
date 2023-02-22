package gcp

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

type Secret *secretmanagerpb.Secret
// FetchSecret will fetch a secret by name for a given project
func FetchSecret(ctx context.Context, projectID string, name string) ([]byte, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to setup secret client: %v", err)
	}
	defer client.Close()
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, name),
	}
	result, err := client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret %s version: %v", name, err)
	}
	return result.Payload.Data, nil
}

func WriteSecret(ctx context.Context, projectID string, name string, sercretValue []byte) (error) {
	client, err  := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup secret client: %v", err)
	}
	defer client.Close()
	createRequest := &secretmanagerpb.CreateSecretRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
		SecretId: name,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}
	_, err = client.CreateSecret(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("failed to create secret: %v", err)
	}

	addVersionRequest := &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s/secrets/%s", projectID, name),
		Payload: &secretmanagerpb.SecretPayload{
			Data: sercretValue,
		},
	}

	_, err = client.AddSecretVersion(ctx, addVersionRequest)
	if err != nil {
		return fmt.Errorf("failed to create secret version: %v", err)
	}

	return nil
}