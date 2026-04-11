package pluginsdk

import (
	"os/exec"
	"testing"

	plugin "github.com/hashicorp/go-plugin"
)

func TestNewClientConfigManagesPluginProcesses(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo")

	cfg := NewClientConfig(cmd)
	if cfg == nil {
		t.Fatalf("expected client config")
	}
	if cfg.Cmd != cmd {
		t.Fatalf("expected command to be preserved")
	}
	if !cfg.Managed {
		t.Fatalf("expected plugin clients to be managed for shutdown cleanup")
	}
	if len(cfg.AllowedProtocols) != 1 || cfg.AllowedProtocols[0] != plugin.ProtocolGRPC {
		t.Fatalf("expected grpc to be the only allowed protocol, got %#v", cfg.AllowedProtocols)
	}
}
