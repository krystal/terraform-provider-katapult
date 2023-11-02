package v6provider

import (
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
	"github.com/krystal/go-katapult/namegenerator"
)

type Meta struct {
	Client *katapult.Client
	Core   *core.Client
	Logger hclog.Logger

	GeneratedNamePrefix  string
	SkipTrashObjectPurge bool

	// Raw provider attribute string values
	confAPIKey       string
	confDataCenter   string
	confOrganization string

	// Internal cache of shallow lookup reference objects
	DataCenterRef   core.DataCenterRef
	OrganizationRef core.OrganizationRef
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

func NewMeta(
	apiKey string,
	datacenter string,
	org string,
	skipTrashObjectPurge *bool,
	logLevel string,
	generatedNamePrefix string,
	httpClient *http.Client,
	version string,
	terraformVersion string,
) (*Meta, error) {
	m := &Meta{
		Logger: hclog.New(&hclog.LoggerOptions{
			Name: "katapult",
			Level: hclog.LevelFromString(
				stringOrEnv(
					logLevel,
					"KATAPULT_LOG_LEVEL",
				),
			),
			TimeFormat: "2006/01/02 15:04:05",
		}),
		confAPIKey: stringOrEnv(
			apiKey,
			"KATAPULT_API_KEY",
		),
		confDataCenter: stringOrEnv(
			datacenter,
			"KATAPULT_DATA_CENTER",
		),
		confOrganization: stringOrEnv(
			org,
			"KATAPULT_ORGANIZATION",
		),
		SkipTrashObjectPurge: boolOrEnv(
			skipTrashObjectPurge,
			"KATAPULT_SKIP_TRASH_OBJECT_PURGE",
		),
		GeneratedNamePrefix: generatedNamePrefix,
	}

	if m.GeneratedNamePrefix == "" {
		m.GeneratedNamePrefix = defaultGeneratedNamePrefix
	}

	opts := []katapult.Option{
		katapult.WithAPIKey(m.confAPIKey),
		katapult.WithUserAgent(
			userAgent(
				"terraform-provider-katapult",
				terraformVersion,
				version,
			),
		),
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	opts = append(opts, katapult.WithHTTPClient(httpClient))

	// Debug override of API URL for internal testing purposes.
	if apiURL := os.Getenv("KATAPULT_TF_DEBUG_API_URL"); apiURL != "" {
		u, err := url.Parse(apiURL)
		if err != nil {
			return nil, err
		}

		opts = append(opts, katapult.WithBaseURL(u))
	}

	c, err := katapult.New(opts...)
	if err != nil {
		return nil, err
	}

	rhc := newRetryableHTTPClient(httpClient, m.Logger)
	c.HTTPClient = rhc.StandardClient()

	m.Client = c
	m.Core = core.New(m.Client)

	m.OrganizationRef = core.OrganizationRef{
		SubDomain: m.confOrganization,
	}
	m.DataCenterRef = core.DataCenterRef{
		Permalink: m.confDataCenter,
	}

	return m, nil
}
