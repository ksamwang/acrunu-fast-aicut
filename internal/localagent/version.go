package localagent

import "runtime"

const (
	AppIdentifier   = "acrunu-fastcut-local-agent"
	ProtocolVersion = 1
)

func platformIdentifier() string {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return "windows-x64"
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}
