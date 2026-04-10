package pluginapi

import "fmt"

func ErrUnknownNode(nodeID string) error {
	return fmt.Errorf("unknown plugin node %q", nodeID)
}
