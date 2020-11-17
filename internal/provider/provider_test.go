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
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/lithammer/dedent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAccResourceNamePrefix = "tf-acc-test"
	testAccProviderVersion    = "0.0.999"
)

var testAccDataCenter = map[string]string{
	"id":           "loc_gTvEnqqnKohbFBJR",
	"name":         "Netwise",
	"permalink":    "netwise",
	"country_id":   "ctry_vDAzWmgGkPoyWMET",
	"country_name": "United Kingdom",
}

func testAccPreCheck(t *testing.T) {
	ctx := context.TODO()

	anyMissing := false
	envVars := []string{
		"KATAPULT_API_URL",
		"KATAPULT_API_KEY",
		"KATAPULT_ORGANIZATION_ID",
		"KATAPULT_DATA_CENTER_ID",
	}
	for _, name := range envVars {
		if os.Getenv(name) == "" {
			anyMissing = true
			t.Errorf(
				"%s environment variable must be set acceptance tests", name,
			)
		}
	}
	if anyMissing {
		t.Fatal("acceptance tests cannot run due to missing configuration")
	}

	if testAccDataCenter["id"] != os.Getenv("KATAPULT_DATA_CENTER_ID") {
		t.Fatalf(
			"Acceptance tests require KATAPULT_DATA_CENTER_ID "+
				"set to \"%s\" (%s)",
			testAccDataCenter["id"], testAccDataCenter["name"],
		)
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
	conf := &Config{Version: testAccProviderVersion}

	if r != nil {
		conf.Transport = r
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
	meta := p.Meta().(*Meta)

	return &TestTools{
		T:                 t,
		Recorder:          r,
		Meta:              meta,
		ProviderFactories: factories,
		Cleanup:           stop,
	}
}

func (tt *TestTools) ResourceName(name string) string {
	if tt.RandID == "" {
		tt.RandID = testDataRandID(tt.T, tt.Recorder)
	}

	return fmt.Sprintf("%s-%s-%s", testAccResourceNamePrefix, name, tt.RandID)
}

func dedentf(format string, a ...interface{}) string {
	return dedent.Dedent(fmt.Sprintf(format, a...))
}

func testDataFilePath(t *testing.T, suffix string) string {
	baseName := filepath.FromSlash(t.Name())
	baseName = strings.TrimPrefix(baseName, "TestAccKatapult")

	if suffix != "" {
		baseName += suffix
	}

	return filepath.Join(".", "testdata", baseName)
}

func testDataRandID(
	t *testing.T,
	r *recorder.Recorder,
) string {
	randPath := testDataFilePath(t, ".rand_id")
	rand := acctest.RandStringFromCharSet(12, acctest.CharSetAlphaNum)

	if r.Mode() == recorder.ModeReplaying {
		data, err := ioutil.ReadFile(randPath)
		if err != nil {
			t.Fatal(fmt.Errorf("missing rand required for VCR replay: %w", err))
		}
		rand = string(bytes.TrimSpace(data))
	} else if r.Mode() == recorder.ModeRecording {
		err := os.MkdirAll(filepath.Dir(randPath), 0o755)
		if err != nil {
			t.Fatal(fmt.Errorf("failed to write rand VCR resource ID: %w", err))
		}

		err = ioutil.WriteFile(randPath, []byte(rand), 0o644) //nolint:gosec
		if err != nil {
			t.Fatal(fmt.Errorf("failed to write rand VCR resource ID: %w", err))
		}
	}

	return rand
}

func newVCRRecorder(t *testing.T) (*recorder.Recorder, func()) {
	cassettePath := testDataFilePath(t, ".cassette")

	mode := recorder.ModeReplaying
	var transport http.RoundTripper

	vcrMode := strings.ToLower(os.Getenv("VCR"))
	switch vcrMode {
	case "replay", "play":
		// Prevent real requests when explicitly set to replay mode
		transport = &stopRequests{}
	case "record", "rec":
		mode = recorder.ModeRecording
	case "disabled", "off", "no", "0":
		mode = recorder.ModeDisabled
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

//
// Tests
//

func TestProvider(t *testing.T) {
	pf := New(&Config{Version: testAccProviderVersion})
	err := pf().InternalValidate()
	require.NoError(t, err)
}
