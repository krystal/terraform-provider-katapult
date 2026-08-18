package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchAllOrganizationVirtualMachinesPaginationRequestAndSorting(t *testing.T) {
	t.Parallel()

	var listRequests atomic.Int32
	var detailRequests atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/organizations/organization/virtual_machines":
			listRequests.Add(1)
			if r.URL.Query().Get("organization[sub_domain]") != "test-org" ||
				r.URL.Query().Get("per_page") != "100" {
				http.NotFound(w, r)
				return
			}
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{
					"virtual_machines":[{"id":"vm_z","name":"Zulu"}],
					"pagination":{"total_pages":2}
				}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{
					"virtual_machines":[{"id":"vm_a","name":"Alpha"}],
					"pagination":{"total_pages":2}
				}`)
			default:
				http.Error(w, "unexpected page", http.StatusNotFound)
			}
		case "/virtual_machines/virtual_machine":
			detailRequests.Add(1)
			http.Error(w, "detail fan-out is forbidden", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})

	virtualMachines, err := fetchAllOrganizationVirtualMachines(
		context.Background(),
		&Meta{Core: client, confOrganization: "test-org"},
	)
	require.NoError(t, err)
	require.Len(t, virtualMachines, 2)
	assert.Equal(t, "vm_a", *virtualMachines[0].Id)
	assert.Equal(t, "vm_z", *virtualMachines[1].Id)
	assert.Equal(t, int32(2), listRequests.Load())
	assert.Zero(t, detailRequests.Load())
}

func TestFetchAllOrganizationVirtualMachinesEmptyAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machines":[],"pagination":{"total_pages":1}
			}`)
		})
		virtualMachines, err := fetchAllOrganizationVirtualMachines(
			context.Background(), &Meta{Core: client},
		)
		require.NoError(t, err)
		assert.NotNil(t, virtualMachines)
		assert.Empty(t, virtualMachines)
	})

	for _, test := range []struct {
		name        string
		contentType string
		status      int
		body        string
		want        string
	}{
		{
			name: "missing ID", contentType: "application/json", status: http.StatusOK,
			body: `{"virtual_machines":[{"name":"missing"}],"pagination":{"total_pages":1}}`,
			want: "virtual machine on page 1 at index 0 has no ID",
		},
		{
			name: "API error", contentType: "application/json",
			status: http.StatusInternalServerError,
			body:   `{"error":{"code":"broken","description":"VM listing failed"}}`,
			want:   "broken: VM listing failed",
		},
		{
			name: "empty successful response", contentType: "text/plain",
			status: http.StatusOK, want: "unexpected empty response",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})
			_, err := fetchAllOrganizationVirtualMachines(
				context.Background(), &Meta{Core: client},
			)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestVirtualMachineSummaryDataSourceModelNullablesAndEmptyIPs(t *testing.T) {
	t.Parallel()

	virtualMachine := core.GetOrganizationVirtualMachines200ResponseVirtualMachines{
		Id:          ptr("vm_test"),
		Name:        ptr("Test"),
		Hostname:    ptr("test"),
		Fqdn:        ptr("test.example.test"),
		IpAddresses: &[]core.GetOrganizationVirtualMachinesPartIPAddresses{{Address: ptr("192.0.2.1")}},
	}
	virtualMachine.Package.Set(core.GetOrganizationVirtualMachinesPartPackage{Name: ptr("ROCK-1")})
	model := virtualMachineSummaryDataSourceModel(&virtualMachine)
	assert.Equal(t, types.StringValue("vm_test"), model.ID)
	assert.Equal(t, types.StringValue("Test"), model.Name)
	assert.Equal(t, types.StringValue("test"), model.Hostname)
	assert.Equal(t, types.StringValue("test.example.test"), model.FQDN)
	assert.Equal(t, types.StringValue("ROCK-1"), model.PackageName)
	require.Len(t, model.IPAddresses.Elements(), 1)
	assert.Equal(t, types.StringValue("192.0.2.1"), model.IPAddresses.Elements()[0])

	missing := core.GetOrganizationVirtualMachines200ResponseVirtualMachines{Id: ptr("vm_empty")}
	missing.Package.SetNull()
	missingModel := virtualMachineSummaryDataSourceModel(&missing)
	assert.True(t, missingModel.Name.IsNull())
	assert.True(t, missingModel.Hostname.IsNull())
	assert.True(t, missingModel.FQDN.IsNull())
	assert.True(t, missingModel.PackageName.IsNull())
	assert.False(t, missingModel.IPAddresses.IsNull())
	assert.Empty(t, missingModel.IPAddresses.Elements())
}

func TestAccKatapultDataSourceVirtualMachines_all(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName("collection")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		// This test covers the collection read; avoid unrelated managed VM
		// refreshes during the framework's plan-stability passes.
		AdditionalCLIOptions: &resource.AdditionalCLIOptions{
			Plan: resource.PlanOptions{NoRefresh: true},
		},
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "first" {}

					resource "katapult_ip" "second" {
					  depends_on = [katapult_virtual_machine.first]
					}

					resource "katapult_virtual_machine" "first" {
					  name          = "%s"
					  hostname      = "%s"
					  package       = "rock-1"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.first.id]
					  system_disk   = {}
					}

					resource "katapult_virtual_machine" "second" {
					  name          = "%s"
					  hostname      = "%s"
					  package       = "rock-1"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.second.id]
					  system_disk   = {}

					  depends_on = [katapult_virtual_machine.first]
					}

					data "katapult_virtual_machines" "all" {
					  depends_on = [
					    katapult_virtual_machine.first,
					    katapult_virtual_machine.second,
					  ]
					}`,
					name+"-first", name+"-first-host",
					name+"-second", name+"-second-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machines.all", "virtual_machines.#", "2",
					),
					testAccCheckVirtualMachineCollectionContains(
						"data.katapult_virtual_machines.all", "katapult_virtual_machine.first",
					),
					testAccCheckVirtualMachineCollectionContains(
						"data.katapult_virtual_machines.all", "katapult_virtual_machine.second",
					),
					testAccCheckVirtualMachineCollectionSorted(
						"data.katapult_virtual_machines.all",
					),
				),
			},
		},
	})
}

func testAccCheckVirtualMachineCollectionContains(
	dataSourceAddress string,
	virtualMachineAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}
		virtualMachine, ok := state.RootModule().Resources[virtualMachineAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", virtualMachineAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["virtual_machines.#"])
		for i := range count {
			prefix := fmt.Sprintf("virtual_machines.%d.", i)
			if dataSource.Primary.Attributes[prefix+"id"] != virtualMachine.Primary.ID {
				continue
			}
			for _, attribute := range []string{
				"name", virtualMachineHostnameAttributeName, "fqdn",
			} {
				got := dataSource.Primary.Attributes[prefix+attribute]
				want := virtualMachine.Primary.Attributes[attribute]
				if attribute == virtualMachineHostnameAttributeName {
					want = strings.ToLower(want)
				}
				if got != want {
					return fmt.Errorf(
						"Virtual Machine %s %s = %q, want %q",
						virtualMachine.Primary.ID, attribute, got, want,
					)
				}
			}
			if dataSource.Primary.Attributes[prefix+"package_name"] == "" {
				return fmt.Errorf(
					"Virtual Machine %s has an empty package_name", virtualMachine.Primary.ID,
				)
			}
			return nil
		}

		return fmt.Errorf(
			"Virtual Machine %s not found in %s", virtualMachine.Primary.ID, dataSourceAddress,
		)
	}
}

func testAccCheckVirtualMachineCollectionSorted(
	dataSourceAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["virtual_machines.#"])
		previous := ""
		for i := range count {
			id := dataSource.Primary.Attributes[fmt.Sprintf("virtual_machines.%d.id", i)]
			if previous != "" && id < previous {
				return fmt.Errorf(
					"Virtual Machines not sorted by ID: %q appears after %q", id, previous,
				)
			}
			previous = id
		}
		return nil
	}
}
