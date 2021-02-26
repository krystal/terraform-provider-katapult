package provider

import (
	"context"

	"github.com/hashicorp/go-hclog"
	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/krystal/go-katapult/pkg/namegenerator"
)

type Meta struct {
	Client *katapult.Client
	Logger hclog.Logger

	GeneratedNamePrefix string

	// Raw provider attribute string values
	confAPIKey       string
	confDataCenter   string
	confOrganization string

	// Internal cache of shallow lookup reference objects
	dataCenterRef   *katapult.DataCenter
	organizationRef *katapult.Organization

	// Internal cache of fully populated objects
	dataCenter   *katapult.DataCenter
	organization *katapult.Organization
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

// DataCenterRef returns a lookup reference for the data center, meaning all
// fields are empty except ID or Permalink. This should only be used for methods
// which take a *katapult.DataCenter as a reference of where to perform an
// operation.
func (m *Meta) DataCenterRef() *katapult.DataCenter {
	if m.dataCenterRef != nil {
		return m.dataCenterRef
	}
	m.dataCenterRef, _ = katapult.NewDataCenterLookup(m.confDataCenter)

	return m.dataCenterRef
}

func (m *Meta) DataCenter(ctx context.Context) (*katapult.DataCenter, error) {
	if m.dataCenter != nil {
		return m.dataCenter, nil
	}

	idOrPermalink := m.dataCenterRef.ID
	if idOrPermalink == "" {
		idOrPermalink = m.dataCenterRef.Permalink
	}

	dc, _, err := m.Client.DataCenters.Get(ctx, idOrPermalink)
	if err != nil {
		return nil, err
	}

	m.dataCenter = dc

	return m.dataCenter, nil
}

func (m *Meta) DataCenterID(ctx context.Context) (string, error) {
	if m.DataCenterRef().ID != "" {
		return m.DataCenterRef().ID, nil
	}

	dc, err := m.DataCenter(ctx)
	if err != nil {
		return "", err
	}

	return dc.ID, nil
}

// OrganizationRef returns a lookup reference for the data center, meaning all
// fields are empty except ID or SubDomain. This should only be used for methods
// which take a *katapult.Organization as a reference of where to perform an
// operation.
func (m *Meta) OrganizationRef() *katapult.Organization {
	if m.organizationRef != nil {
		return m.organizationRef
	}
	m.organizationRef, _ = katapult.NewOrganizationLookup(m.confOrganization)

	return m.organizationRef
}

func (m *Meta) Organization(
	ctx context.Context,
) (*katapult.Organization, error) {
	if m.organization != nil {
		return m.organization, nil
	}

	idOrPermalink := m.organizationRef.ID
	if idOrPermalink == "" {
		idOrPermalink = m.organizationRef.SubDomain
	}

	dc, _, err := m.Client.Organizations.Get(ctx, idOrPermalink)
	if err != nil {
		return nil, err
	}

	m.organization = dc

	return m.organization, nil
}

func (m *Meta) OrganizationID(ctx context.Context) (string, error) {
	if m.OrganizationRef().ID != "" {
		return m.OrganizationRef().ID, nil
	}

	dc, err := m.Organization(ctx)
	if err != nil {
		return "", err
	}

	return dc.ID, nil
}
