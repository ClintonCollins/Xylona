package supervisor

import (
	"github.com/ClintonCollins/Xylona/internal/processenv"
)

func buildChildEnvironment(runtime Runtime, sourceEnv []string) []string {
	return processenv.Build(string(runtime), sourceEnv)
}

func appendLaunchEnvironment(baseEnv []string, launchEnv map[string]string) []string {
	return processenv.Append(baseEnv, launchEnv)
}
