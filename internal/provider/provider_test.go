package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAccResourceNamePrefix = "tf-acc-test"
	testAccProviderVersion    = "0.0.999"
)

func testAccPreCheck(t *testing.T) {
	ctx := context.TODO()

	anyMissing := false
	envVars := []string{
		"KATAPULT_API_KEY",
		"KATAPULT_ORGANIZATION",
		"KATAPULT_DATA_CENTER",
	}
	for _, name := range envVars {
		if os.Getenv(name) == "" {
			anyMissing = true
			t.Errorf(
				"%s environment variable must be set for acceptance tests",
				name,
			)
		}
	}
	if anyMissing {
		t.Fatal("acceptance tests cannot run due to missing configuration")
	}

	provider, err := providerFactories(nil)["katapult"]()
	require.NoError(t, err)

	diags := provider.Configure(
		ctx, terraform.NewResourceConfigRaw(nil),
	)
	if diags.HasError() {
		t.Fatal(diags[0].Summary)
	}
}

type providerFactoryList map[string]func() (*schema.Provider, error)

func providerFactories(
	r *recorder.Recorder,
) providerFactoryList {
	conf := &Config{
		Version:             testAccProviderVersion,
		GeneratedNamePrefix: testAccResourceNamePrefix,
	}

	if r != nil {
		conf.HTTPClient = &http.Client{Transport: r}
	}

	return providerFactoryList{
		//nolint:unparam
		"katapult": func() (*schema.Provider, error) {
			pf := New(conf)

			return pf(), nil
		},
	}
}

type stopRequests struct{}

func (s *stopRequests) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
	}, errors.New("real HTTP(S) requests are disabled")
}

type testTools struct {
	T                 *testing.T
	Ctx               context.Context
	Recorder          *recorder.Recorder
	Meta              *Meta
	ProviderFactories providerFactoryList
	randID            string
}

func newTestTools(t *testing.T) *testTools {
	r := newVCRRecorder(t)
	factories := providerFactories(r)
	ctx := context.Background()

	p, err := factories["katapult"]()
	require.NoError(t, err)
	d := p.Configure(ctx, terraform.NewResourceConfigRaw(nil))
	if d.HasError() {
		t.Fatalf("failed to configure client: %+v", d)
	}
	m := p.Meta().(*Meta)

	return &testTools{
		T:                 t,
		Ctx:               ctx,
		Recorder:          r,
		Meta:              m,
		ProviderFactories: factories,
	}
}

// ResourceName returns the name of a resource in the test provider, for the
// purpose of having unique names which can easily be identified as belonging to
// the acceptance test suite.
func (tt *testTools) ResourceName(name ...string) string {
	if len(name) == 0 && strings.HasPrefix(tt.T.Name(), "TestAcc") {
		if parts := strings.Split(tt.T.Name(), "_"); len(parts) > 1 {
			if strings.Contains(parts[0], "DataSource") {
				name = append(name, "data-source")
			}

			name = append(name, parts[1:]...)
		}
	}

	if len(name) == 0 {
		name = []string{"default"}
	}

	nameStr := strings.Join(name, "-")

	return fmt.Sprintf("%s-%s-%s",
		testAccResourceNamePrefix, nameStr, tt.RandID(),
	)
}

func (tt *testTools) RandID() string {
	if tt.randID != "" {
		return tt.randID
	}

	rand := acctest.RandString(12)
	if tt.Recorder == nil {
		return rand
	}

	randIDFile := testDataFilePath(tt.T, ".cassette.rand_id")
	if tt.Recorder.Mode() == recorder.ModeReplaying {
		data, err := os.ReadFile(randIDFile)
		require.NoError(tt.T, err, "missing rand required for VCR replay")
		rand = string(bytes.TrimSpace(data))
	} else if tt.Recorder.Mode() == recorder.ModeRecording {
		err := os.MkdirAll(filepath.Dir(randIDFile), 0o755)
		require.NoError(tt.T, err, "failed to write rand VCR resource ID")

		err = os.WriteFile(randIDFile, []byte(rand), 0o644) //nolint:gosec
		require.NoError(tt.T, err, "failed to write rand VCR resource ID")
	}

	return rand
}

func testDataFilePath(t *testing.T, suffix string) string {
	baseName := filepath.FromSlash(t.Name())
	baseName = strings.TrimPrefix(baseName, "TestAccKatapult")

	if suffix != "" {
		baseName += suffix
	}

	return filepath.Join(".", "testdata", baseName)
}

func exampleResourceConfig(t *testing.T, name string) string {
	t.Helper()

	filename := filepath.Join(
		"..", "..", "examples", "resources", name, "resource.tf",
	)
	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	return string(data)
}

func vcrMode() recorder.Mode {
	switch strings.ToLower(os.Getenv("VCR")) {
	case "disabled", "off", "no", "0":
		return recorder.ModeDisabled
	case "record", "rec":
		return recorder.ModeRecording
	default:
		// Prevent real requests unless VCR is explicitly set to record mode.
		return recorder.ModeReplaying
	}
}

func newVCRRecorder(t *testing.T) *recorder.Recorder {
	cassettePath := testDataFilePath(t, ".cassette")

	var transport http.RoundTripper
	mode := vcrMode()

	switch mode {
	case recorder.ModeDisabled:
		return nil
	case recorder.ModeReplaying:
		transport = &stopRequests{}
	case recorder.ModeRecording:
		// Use the default transport.
	}

	r, err := recorder.NewAsMode(cassettePath, mode, transport)
	if err != nil {
		t.Fatal(err)
	}

	r.AddFilter(func(i *cassette.Interaction) error {
		delete(i.Request.Headers, "Authorization")

		return nil
	})

	t.Cleanup(func() {
		assert.NoError(t, r.Stop())
	})

	return r
}

//
// Terraform TestCheckFunc helpers
//

func testCheckGeneratedResourceName(
	name string,
	key string,
) resource.TestCheckFunc {
	return resource.TestMatchResourceAttr(
		name, key,
		regexp.MustCompile(
			fmt.Sprintf(
				"^%s-.+-.+$",
				regexp.QuoteMeta(testAccResourceNamePrefix),
			),
		),
	)
}

func testCheckGeneratedHostnameName(
	name string,
	key string,
) resource.TestCheckFunc {
	return resource.TestMatchResourceAttr(
		name, key,
		regexp.MustCompile(
			fmt.Sprintf(
				"^%s-.+-.+-.+$",
				regexp.QuoteMeta(testAccResourceNamePrefix),
			),
		),
	)
}

//
// Provider Tests
//

func TestProvider(t *testing.T) {
	pf := New(&Config{Version: testAccProviderVersion})
	err := pf().InternalValidate()
	require.NoError(t, err)
}
