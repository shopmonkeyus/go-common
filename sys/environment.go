package sys

import (
	"errors"
	"fmt"
	"os"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Environment string

const (
	Stable  Environment = "stable"
	Sandbox Environment = "sandbox"
	Edge    Environment = "edge"
	Dev     Environment = "dev"
	RC      Environment = "rc"
)

func (e Environment) String() string {
	return string(e)
}

func NewEnvironment(env string) (Environment, error) {
	return parseEnvironment(env)
}

func parseEnvironment(env string) (Environment, error) {
	switch strings.ToLower(env) {
	case "stable":
		return Stable, nil
	case "sandbox":
		return Sandbox, nil
	case "edge":
		return Edge, nil
	case "rc":
		return RC, nil
	case "dev", "":
		return Dev, nil
	default:
		return "", errors.New("invalid environment")
	}
}

func GetEnvironment() (Environment, error) {
	env := os.Getenv("SM_ENV")
	return parseEnvironment(env)
}

func MustGetEnvironment() Environment {
	env, err := GetEnvironment()
	if err != nil {
		panic(err)
	}
	return env
}

// GetAPIURLFromJWT extracts the API URL from a JWT token
func GetAPIURLFromJWT(jwtString string) (string, error) {
	p := jwt.NewParser(jwt.WithoutClaimsValidation())
	var claims jwt.RegisteredClaims
	tokens, _, err := p.ParseUnverified(jwtString, &claims)
	if err != nil {
		return "", fmt.Errorf("failed to parse jwt: %w", err)
	}
	iss, err := tokens.Claims.GetIssuer()
	if err != nil {
		return "", fmt.Errorf("failed to get issuer from jwt: %w", err)
	}
	if iss == "https://shopmonkey.io" {
		// support for legacy tokens
		iss = "https://api.shopmonkey.cloud"
	}
	return iss, nil
}
