package provider

import (
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
	"github.com/krystal/go-katapult/namegenerator"
	corenext "github.com/krystal/go-katapult/next/core"
)

type Meta struct {
	Client   *katapult.Client
	Core     *core.Client
	CoreNext corenext.ClientWithResponsesInterface
	Logger   hclog.Logger

	GeneratedNamePrefix  string
	SkipTrashObjectPurge bool
	testMode             bool

	// Raw provider attribute string values
	confAPIKey       string
	confDataCenter   string
	confOrganization string

	// Internal cache of shallow lookup reference objects
	DataCenterRef   core.DataCenterRef
	OrganizationRef core.OrganizationRef
}

// stateChangeDelay returns zero in replay mode, or d otherwise.
func (m *Meta) stateChangeDelay(d time.Duration) time.Duration {
	if m.testMode {
		return 0
	}

	return d
}

// stateChangePollInterval returns a short fixed interval in replay mode so
// StateChangeConf does not use its production exponential backoff.
func (m *Meta) stateChangePollInterval() time.Duration {
	if m.testMode {
		return time.Millisecond
	}

	return 0
}

func (m *Meta) UseOrGenerateName(name string) string {
	switch {
	case name != "":
		return name
	case m.GeneratedNamePrefix == "":
		return namegenerator.RandomName()
	default:
		return namegenerator.RandomName(m.GeneratedNamePrefix)
	}
}

func (m *Meta) UseOrGenerateHostname(hostname string) string {
	switch {
	case hostname != "":
		return hostname
	case m.GeneratedNamePrefix == "":
		return namegenerator.RandomHostname()
	default:
		return m.GeneratedNamePrefix + "-" + namegenerator.RandomHostname()
	}
}
