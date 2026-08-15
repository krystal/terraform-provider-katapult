package v6provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorHTTPDoer struct {
	err error
}

func (d errorHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return nil, d.err
}

//nolint:lll // Compact table rows keep boot-selection scenarios comparable.
func TestSelectBootDiskAssignment(t *testing.T) {
	t.Parallel()

	trueValue := true
	falseValue := false
	disk := func(id string, boot *bool) core.GetVirtualMachineDisks200ResponseDisks {
		return core.GetVirtualMachineDisks200ResponseDisks{Boot: boot, Disk: &core.GetVirtualMachineDisksPartDisk{Id: &id}}
	}
	tests := []struct {
		name        string
		attachments []core.GetVirtualMachineDisks200ResponseDisks
		prior       string
		want        string
		ok          bool
	}{
		{name: "explicit true", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("data", &falseValue), disk("boot", &trueValue)}, want: "boot", ok: true},
		{name: "prior fallback", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("boot", &falseValue), disk("data", &falseValue)}, prior: "boot", want: "boot", ok: true},
		{name: "one nil fallback", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("boot", nil), disk("data", &falseValue)}, want: "boot", ok: true},
		{name: "missing prior", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("data", &falseValue)}, prior: "missing", ok: false},
		{name: "no boot evidence", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("data", &falseValue)}, ok: false},
		{name: "ambiguous nil", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("a", nil), disk("b", nil)}, ok: false},
		{name: "multiple explicit", attachments: []core.GetVirtualMachineDisks200ResponseDisks{disk("a", &trueValue), disk("b", &trueValue)}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selected, ok := selectBootDiskAssignment(test.attachments, test.prior)
			assert.Equal(t, test.ok, ok)
			if test.ok {
				assert.Equal(t, test.want, *selected.Disk.Id)
			}
		})
	}
}

func TestWaitForDiskSizeRequiresAPIConvergence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "target observed", size: 30},
		{name: "task success without size change", size: 20, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{"disk":{"id":"disk_test","size_in_gb":%d}}`, test.size))
			})
			err := waitForDiskSize(
				context.Background(),
				&Meta{Core: client, testMode: true},
				"disk_test",
				30,
				20*time.Millisecond,
			)
			if test.wantErr {
				require.ErrorContains(t, err, "waiting for disk disk_test size 30 GB")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFetchAllVMDisksPaginates(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet ||
			r.URL.Path != "/virtual_machines/virtual_machine/disks" ||
			r.URL.Query().Get("virtual_machine[id]") != "vm_test" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("page") {
		case "1":
			writeTestJSON(w, http.StatusOK, `{
				"disks":[{"disk":{"id":"disk_z"}}],
				"pagination":{"total_pages":2}
			}`)
		case "2":
			writeTestJSON(w, http.StatusOK, `{
				"disks":[{"disk":{"id":"disk_a"}}],
				"pagination":{"total_pages":2}
			}`)
		default:
			http.Error(w, "unexpected page", http.StatusNotFound)
		}
	})

	disks, err := fetchAllVMDisks(
		context.Background(), &Meta{Core: client}, "vm_test",
	)
	require.NoError(t, err)
	require.Len(t, disks, 2)
	require.Equal(t, int32(2), requests.Load())
	require.Equal(t, "disk_z", *disks[0].Disk.Id)
	require.Equal(t, "disk_a", *disks[1].Disk.Id)
}

func TestPurgeTrashObjectPreservesTransportErrorWithNilResponse(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("transport unavailable")
	client, err := core.NewClientWithResponses(
		"https://api.example.test",
		"test-token",
		core.WithHTTPClient(errorHTTPDoer{err: wantErr}),
	)
	require.NoError(t, err)

	err = purgeTrashObjectByObjectID(
		context.Background(),
		&Meta{Core: client, testMode: true},
		time.Second,
		"vm_test",
	)

	require.ErrorIs(t, err, wantErr)
}
