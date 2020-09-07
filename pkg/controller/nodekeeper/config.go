package nodekeeper

import (
	"fmt"
	"time"
)

type nodeKeeperConfig struct {
	NodeDrain nodeDrain `yaml:"nodeDrain"`
}

type nodeDrain struct {
	Timeout int `yaml:"timeOut"`
}

func (nkc *nodeKeeperConfig) IsValid() error {
	if nkc.NodeDrain.Timeout < 0 || nkc.NodeDrain.Timeout > 120 {
		return fmt.Errorf("Config nodeDrain timeOut is invalid")
	}
	return nil
}

func (nkc *nodeKeeperConfig) GetNodeDrainDuration() time.Duration {
	return time.Duration(nkc.NodeDrain.Timeout) * time.Minute
}
