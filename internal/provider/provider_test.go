package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
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

func providerFactories(
	r *recorder.Recorder,
) map[string]func() (*schema.Provider, error) {
	conf := &Config{
		Version:             testAccProviderVersion,
		GeneratedNamePrefix: testAccResourceNamePrefix,
	}

	if r != nil {
		conf.HTTPClient = &http.Client{Transport: r}
	}

	return map[string]func() (*schema.Provider, error){
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

type TestTools struct {
	T                 *testing.T
	Recorder          *recorder.Recorder
	Meta              *Meta
	ProviderFactories map[string]func() (*schema.Provider, error)
	Cleanup           func()
	RandID            string
}

func NewTestTools(t *testing.T) *TestTools {
	r, stop := newVCRRecorder(t)
	factories := providerFactories(r)

	p, err := factories["katapult"]()
	require.NoError(t, err)
	d := p.Configure(context.TODO(), terraform.NewResourceConfigRaw(nil))
	if d.HasError() {
		t.Fatalf("failed to configure client: %+v", d)
	}
	m := p.Meta().(*Meta)

	return &TestTools{
		T:                 t,
		Recorder:          r,
		Meta:              m,
		ProviderFactories: factories,
		Cleanup:           stop,
	}
}

func (tt *TestTools) ResourceName(name string) string {
	if tt.RandID == "" {
		tt.RandID = testVCRRecorderRandID(tt.T, tt.Recorder)
	}

	return fmt.Sprintf("%s-%s-%s", testAccResourceNamePrefix, name, tt.RandID)
}

func testDataFilePath(t *testing.T, suffix string) string {
	baseName := filepath.FromSlash(t.Name())
	baseName = strings.TrimPrefix(baseName, "TestAccKatapult")

	if suffix != "" {
		baseName += suffix
	}

	return filepath.Join(".", "testdata", baseName)
}

func newVCRRecorder(t *testing.T) (*recorder.Recorder, func()) {
	cassettePath := testDataFilePath(t, ".cassette")

	var mode recorder.Mode
	var transport http.RoundTripper

	vcrMode := strings.ToLower(os.Getenv("VCR"))
	switch vcrMode {
	case "record", "rec":
		mode = recorder.ModeRecording
	case "disabled", "off", "no", "0":
		mode = recorder.ModeDisabled
	default:
		// Prevent real requests unless VCR is explicitly set to record mode or
		// disabled.
		transport = &stopRequests{}
		mode = recorder.ModeReplaying
	}

	r, err := recorder.NewAsMode(cassettePath, mode, transport)
	if err != nil {
		t.Fatal(err)
	}

	r.AddFilter(func(i *cassette.Interaction) error {
		delete(i.Request.Headers, "Authorization")

		return nil
	})

	stop := func() {
		assert.NoError(t, r.Stop())
	}

	return r, stop
}

func testVCRRecorderRandID(
	t *testing.T,
	r *recorder.Recorder,
) string {
	randIDFile := testDataFilePath(t, ".cassette.rand_id")
	rand := acctest.RandString(12)

	if r.Mode() == recorder.ModeReplaying {
		data, err := ioutil.ReadFile(randIDFile)
		if err != nil {
			t.Fatal(fmt.Errorf("missing rand required for VCR replay: %w", err))
		}
		rand = string(bytes.TrimSpace(data))
	} else if r.Mode() == recorder.ModeRecording {
		err := os.MkdirAll(filepath.Dir(randIDFile), 0o755)
		if err != nil {
			t.Fatal(fmt.Errorf("failed to write rand VCR resource ID: %w", err))
		}

		err = ioutil.WriteFile(randIDFile, []byte(rand), 0o644) //nolint:gosec
		if err != nil {
			t.Fatal(fmt.Errorf("failed to write rand VCR resource ID: %w", err))
		}
	}

	return rand
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
