package sys

import (
	"errors"
	"os"
	"strings"
)

type Environment string

const (
	Stable  Environment = "stable"
	Sandbox Environment = "sandbox"
	Edge    Environment = "edge"
	Dev     Environment = "dev"
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
