package nodekeeper

import "os"

const (
	envVarUpgrading = "UPGRADING"
)

func fakeUpgradeState() bool {
	found := false
	result, found := os.LookupEnv(envVarUpgrading)
	if !found {
		return found
	}

	if result == "true" {
		return true
	}
	return false
}
