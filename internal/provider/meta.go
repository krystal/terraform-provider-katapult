package provider

import (
	"context"

	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/krystal/go-katapult/pkg/namegenerator"
)

type Meta struct {
	Client *katapult.Client
	Ctx    context.Context

	APIURL              string
	APIKey              string
	OrganizationID      string
	DataCenterID        string
	GeneratedNamePrefix string
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

func (m *Meta) Organization() *katapult.Organization {
	return &katapult.Organization{ID: m.OrganizationID}
}

func (m *Meta) DataCenter() *katapult.DataCenter {
	return &katapult.DataCenter{ID: m.DataCenterID}
}
