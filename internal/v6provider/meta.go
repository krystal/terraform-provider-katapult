package v6provider

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"github.com/hashicorp/go-hclog"

	"github.com/krystal/go-katapult"
	core "github.com/krystal/go-katapult/next/core"

	"github.com/krystal/go-katapult/namegenerator"
)

type Meta struct {
	Client *katapult.Client
	Core   core.ClientWithResponsesInterface
	Logger hclog.Logger

	GeneratedNamePrefix  string
	SkipTrashObjectPurge bool

	// Raw provider attribute string values
	confAPIKey       string
	confDataCenter   string
	confOrganization string
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
	logLevel = stringOrEnv(
		logLevel,
		"KATAPULT_LOG_LEVEL",
	)
	if logLevel == "" {
		logLevel = "info"
	}

	m := &Meta{
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:       "katapult",
			Level:      hclog.LevelFromString(logLevel),
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

	serverURL := &url.URL{
		Scheme: "https",
		Host:   "api.katapult.io",
		Path:   "/core/v1",
	}

	// Debug override of API URL for internal testing purposes.
	if apiURL := os.Getenv("KATAPULT_TF_DEBUG_API_URL"); apiURL != "" {
		u, err := url.Parse(apiURL)
		if err != nil {
			return nil, err
		}

		u.Path = "/core/v1"

		serverURL = u
	}

	rhc := newRetryableHTTPClient(httpClient, m.Logger)

	coreClient, err := core.NewClientWithResponses(
		serverURL.String(),
		m.confAPIKey,
		core.WithHTTPClient(rhc.StandardClient()),
		core.WithRequestEditorFn(
			func(_ context.Context, req *http.Request) error {
				req.Header.Set(
					"User-Agent",
					userAgent(
						"terraform-provider-katapult",
						terraformVersion,
						version,
					),
				)

				return nil
			},
		),
	)
	if err != nil {
		return nil, err
	}

	m.Core = coreClient

	return m, nil
}
