package nodekeeper

import "time"

func getGreaterForceDrainDuration(pdb time.Duration, sre time.Duration) time.Duration {
	defaultForceDrain := pdb

	if pdb < sre {
		defaultForceDrain = sre
	}

	return defaultForceDrain
}
