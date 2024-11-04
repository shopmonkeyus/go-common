package env

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/shopmonkeyus/go-common/gcp"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/shopmonkeyus/go-common/sys"
	"github.com/spf13/cobra"
	"google.golang.org/api/iterator"
)

func mustQuote(val string) bool {
	if strings.Contains(val, `"`) {
		return true
	}
	if strings.Contains(val, "\\n") {
		return true
	}
	return false
}

type callback func(key, val string) string

// EncodeOSEnvFunc encodes an environment variable for use in an OS environment using a custom sprintf function.
func EncodeOSEnvFunc(key, val string, fn callback) string {
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "'", "\\'")
	if mustQuote(val) {
		if strings.Contains(val, `"`) {
			val = `'` + val + `'`
		} else {
			val = `"` + val + `"`
		}
	}
	return fn(key, val)
}

// EncodeOSEnv encodes an environment variable for use in an OS environment.
func EncodeOSEnv(key, val string) string {
	return EncodeOSEnvFunc(key, val, func(key, val string) string {
		return fmt.Sprintf(`%s=%s`, key, val)
	})
}

func dequote(s string) string {
	v := s
	if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
		v = strings.TrimLeft(v, "'")
		v = strings.TrimRight(v, "'")
	} else if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		v = strings.TrimLeft(v, `"`)
		v = strings.TrimRight(v, `"`)
	}
	return v
}

type EnvLine struct {
	Key      string `json:"key"`
	Val      string `json:"val"`
	IsSecret bool   `json:"secret,omitempty"`
}

func IsValueSecret(val string) (bool, string) {
	if strings.Contains(val, "${") {
		return true, val[2 : len(val)-1]
	}
	return false, val
}

func ParseEnvValue(key, val string) EnvLine {
	secret, sval := IsValueSecret(val)
	return EnvLine{
		Key:      key,
		Val:      sval,
		IsSecret: secret,
	}
}

// ProcessEnvLine processes an environment variable line and returns an EnvLine struct with the key, value, and secret flag set.
func ProcessEnvLine(env string) EnvLine {
	tok := strings.SplitN(env, "=", 2)
	key := tok[0]
	val := dequote(tok[1])
	return ParseEnvValue(key, val)
}

// ParseEnvFile parses an environment file and returns a list of EnvLine structs.
func ParseEnvFile(filename string) ([]EnvLine, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ParseEnvBuffer(buf)
}

// ParseEnvBuffer parses an environment file from a buffer and returns a list of EnvLine structs.
func ParseEnvBuffer(buf []byte) ([]EnvLine, error) {
	var envs []EnvLine
	if len(buf) > 0 {
		lines := strings.Split(string(buf), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' || !strings.Contains(line, "=") {
				continue
			}
			envs = append(envs, ProcessEnvLine(line))
		}
	}
	return envs, nil
}

type secretCallback func(envs []EnvLine) (bool, []EnvLine, error)

func ProcessSecrets(ctx context.Context, logger logger.Logger, projectID string, env string, group string, callback secretCallback) error {
	name := GetSecretNameForEnv(env, group)
	buf, err := gcp.FetchSecret(ctx, projectID, name)
	if err != nil {
		if !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("failed to fetch secret: %w", err)
		}
	}
	if len(buf) > 0 {
		gz, err := gzip.NewReader(bytes.NewBuffer(buf))
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gz.Close()
		var out bytes.Buffer
		if _, err := io.Copy(&out, gz); err != nil {
			return fmt.Errorf("failed to decompress secret: %w", err)
		}
		buf = out.Bytes()
	}
	envs, err := ParseEnvBuffer(buf)
	if err != nil {
		return fmt.Errorf("failed to parse env buffer: %w", err)
	}
	write, envs, err := callback(envs)
	if err != nil {
		return fmt.Errorf("failed to process secrets: %w", err)
	}
	if !write {
		return nil
	}
	var envstrs []string
	for _, envline := range envs {
		envstrs = append(envstrs, EncodeOSEnv(envline.Key, envline.Val))
	}

	if len(envstrs) == 0 {
		if err := deleteSecret(ctx, projectID, name); err != nil {
			return fmt.Errorf("failed to delete secret: %w", err)
		}
		logger.Info("deleted %s secret since there are no more entries", name)
		return nil
	}

	outbuf := []byte(strings.Join(envstrs, "\n"))

	if !bytes.Equal(outbuf, buf) {
		var out bytes.Buffer
		gz := gzip.NewWriter(&out)
		if _, err := gz.Write(outbuf); err != nil {
			return fmt.Errorf("failed to compress secret: %w", err)
		}
		if err := gz.Flush(); err != nil {
			return fmt.Errorf("failed to flush gzip writer: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
		labels := map[string]string{"piston": "true", "piston-env": env, "piston-group": group}
		if err := writeSecret(ctx, projectID, name, labels, out.Bytes()); err != nil {
			return fmt.Errorf("failed to write secret: %w", err)
		}
		logger.Info("%s secrets updated with %d entries", name, len(envs))
	} else {
		logger.Info("%s secrets not updated because no change detected", name)
	}
	return nil
}

func GetSecretNameForEnv(env string, group string) string {
	return fmt.Sprintf("SM_PISTON_%s_%s", strings.ToUpper(group), strings.ToUpper(env))
}

func deleteSecret(ctx context.Context, projectID string, name string) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup secret client: %w", err)
	}
	defer client.Close()
	deleteRequest := &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", projectID, name),
	}
	if err := client.DeleteSecret(ctx, deleteRequest); err != nil {
		if !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("failed to delete secret: %w", err)
		}
	}
	return nil
}

func writeSecret(ctx context.Context, projectID string, name string, labels map[string]string, sercretValue []byte) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup secret client: %w", err)
	}
	defer client.Close()
	createRequest := &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", projectID),
		SecretId: name,
		Secret: &secretmanagerpb.Secret{
			Labels: labels,
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}
	_, err = client.CreateSecret(ctx, createRequest)

	var exists bool
	if err != nil && strings.Contains(err.Error(), "AlreadyExists") {
		exists = true
	}
	if err != nil && !exists {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	if exists {
		it := client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
			Parent: fmt.Sprintf("projects/%s/secrets/%s", projectID, name),
		})
		for {
			sv, err := it.Next()
			if err == iterator.Done {
				break
			}
			if sv.GetState() == secretmanagerpb.SecretVersion_ENABLED {
				if _, err := client.DestroySecretVersion(ctx, &secretmanagerpb.DestroySecretVersionRequest{
					Name: sv.GetName(),
					Etag: sv.GetEtag(),
				}); err != nil {
					return fmt.Errorf("failed to destroy secret version: %w", err)
				}
			}
		}
	}

	addVersionRequest := &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s/secrets/%s", projectID, name),
		Payload: &secretmanagerpb.SecretPayload{
			Data: sercretValue,
		},
	}

	_, err = client.AddSecretVersion(ctx, addVersionRequest)
	if err != nil {
		return fmt.Errorf("failed to create secret version: %w", err)
	}

	return nil
}

func EncodeOSEnvWithSecret(key, val string) string {
	return fmt.Sprintf("%s=${%s}", key, val)
}

func WriteEnvFile(fn string, envs []EnvLine) error {
	of, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer of.Close()
	for _, el := range envs {
		if el.IsSecret {
			fmt.Fprintln(of, EncodeOSEnvWithSecret(el.Key, el.Val))
		} else {
			fmt.Fprintln(of, EncodeOSEnv(el.Key, el.Val))
		}
	}
	return of.Close()
}

func getEnvFile(dir string, envname string) string {
	return filepath.Join(dir, ".env."+envname)
}

func replaceEnv(environment []string, key string, val string, skipIsExists bool) []string {
	var found bool
	for i, envline := range environment {
		if strings.HasPrefix(envline, key+"=") {
			if skipIsExists {
				return environment
			}
			environment[i] = EncodeOSEnv(key, val)
			found = true
		}
	}
	if !found {
		environment = append(environment, EncodeOSEnv(key, val))
	}
	return environment
}

type GetEnvOptions struct {
	Cmd         *cobra.Command
	Logger      logger.Logger
	Dir         string
	SkipSecrets bool
	SkipOSEnvs  bool
	SetEnv      bool
	EnvName     string
	GroupName   string
}

// GetEnv will return the environment variables for the given environment and group.
func GetEnv(opts GetEnvOptions) ([]string, error) {
	envfile := getEnvFile(opts.Dir, opts.EnvName)

	var environment []string

	if opts.SkipOSEnvs {
		environment = append(environment, fmt.Sprintf("SM_ENV=%s", opts.EnvName))
	} else {
		environment = append(os.Environ(), fmt.Sprintf("SM_ENV=%s", opts.EnvName))
	}

	logger := opts.Logger.WithPrefix("[env]")

	secrets := make(map[string]string)

	if sys.Exists(envfile) {
		logger.Debug("loading environment variables from %s", envfile)
		kv, err := ParseEnvFile(envfile)
		if err != nil {
			logger.Fatal("error parsing env file %s: %s", envfile, err)
		}
		for _, envline := range kv {
			if !envline.IsSecret {
				environment = replaceEnv(environment, envline.Key, envline.Val, true)
			} else if !opts.SkipSecrets {
				var found bool
				// first look inside the existing env file
				for _, envline2 := range kv {
					if envline2.Key == envline.Key && !envline2.IsSecret {
						environment = append(environment, EncodeOSEnv(envline.Key, envline2.Val))
						found = true
						break
					}
				}
				// if not found, look in the os environment
				if !found {
					if val, ok := os.LookupEnv(envline.Key); ok {
						environment = append(environment, EncodeOSEnv(envline.Key, val))
						found = true
					}
				}
				// if not found, add it to the secrets to lookup
				if !found {
					secrets[envline.Key] = envline.Val
				}
			} else {
				environment = append(environment, EncodeOSEnvWithSecret(envline.Key, envline.Val))
			}
		}
	} else if !opts.SkipSecrets && !opts.SkipOSEnvs {
		logger.Debug("no env file found at %s", envfile)
		for _, envline := range os.Environ() {
			kv := ProcessEnvLine(envline)
			if kv.IsSecret {
				secrets[kv.Key] = kv.Val
				logger.Debug("need to map secret: %s to environment: %s", kv.Val, kv.Key)
			}
		}
	}

	if len(secrets) > 0 && !opts.SkipSecrets && opts.Cmd != nil {
		projectId, cleanup, err := gcp.FetchProjectIDUsingCmd(opts.Cmd)
		if err != nil {
			return nil, fmt.Errorf("error getting project id: %w", err)
		}
		defer cleanup()
		if err := ProcessSecrets(context.Background(), logger, projectId, opts.EnvName, opts.GroupName, func(envs []EnvLine) (bool, []EnvLine, error) {
			for name, val := range secrets {
				var found bool
				for _, envline := range envs {
					if envline.Key == val {
						logger.Trace("found secret: %s with key: %s", name, val)
						environment = replaceEnv(environment, name, envline.Val, true)
						found = true
						break
					}
				}
				if !found {
					return false, envs, fmt.Errorf("secret %s not found for environment: %s", name, opts.EnvName)
				}
			}
			return false, envs, nil
		}); err != nil {
			return nil, fmt.Errorf("error processing secrets: %w", err)
		}
	} else {
		logger.Debug("no secrets to process for environment: %s", opts.EnvName)
	}
	if opts.SetEnv {
		for _, envline := range environment {
			kv := ProcessEnvLine(envline)
			os.Setenv(kv.Key, kv.Val)
		}
	}
	return environment, nil
}

// SetEnvFromEnvFile will set the environment variables from an env file.
func SetEnvFromEnvFile(envfile string) error {
	envs, err := ParseEnvFile(envfile)
	if err != nil {
		return err
	}
	for _, kv := range envs {
		os.Setenv(kv.Key, kv.Val)
	}
	return nil
}
