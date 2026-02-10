package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/jimeh/rands"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/buildspec"
	"github.com/krystal/go-katapult/core"
	corenext "github.com/krystal/go-katapult/next/core"
)

func resourceVirtualMachine() *schema.Resource { //nolint:funlen
	return &schema.Resource{
		CreateContext: resourceVirtualMachineCreate,
		ReadContext:   resourceVirtualMachineRead,
		UpdateContext: resourceVirtualMachineUpdate,
		DeleteContext: resourceVirtualMachineDelete,
		CustomizeDiff: resourceVirtualMachineCustomizeDiff,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
		},
		//nolint:lll
		Description: strings.TrimSpace(`

The Virtual Machine resource allows you to create and manage Virtual Machines in Katapult.

~> **Warning:** Deleting a virtual machine resource with Terraform will by default purge the VM from Katapult's trash, permanently deleting it. If you wish to instead keep a deleted VM in the trash, set the` + "`skip_trash_object_purge`" + ` provider option to ` + "`true`" + `. By default, objects in the trash are permanently deleted after 48 hours.

`),
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"hostname": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"fqdn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"package": {
				Type:         schema.TypeString,
				Description:  "Permalink or ID of a Virtual Machine Package.",
				Required:     true,
				ForceNew:     true, // TODO: Add support for changing package
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"disk_template": {
				Type:         schema.TypeString,
				Description:  "Permalink or ID of a Disk Template.",
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"disk_template_options": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"disk": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Description: "Specify one or more disks with custom sizes to " +
					"create and attach to the Virtual Machine during " +
					"creation. First defined disk will be used as the boot " +
					"disk. If no disks are defined, a single disk will be " +
					"created based on the chosen package.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Name of the disk.",
						},
						"size": {
							Type:        schema.TypeInt,
							Required:    true,
							ForceNew:    true,
							Description: "Size of the disk in GB.",
						},
					},
				},
			},
			"ip_address_ids": {
				Type:        schema.TypeSet,
				Description: "One or more IP IDs.",
				Required:    true,
				MinItems:    1,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"ip_addresses": {
				Type:     schema.TypeSet,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"virtual_network_ids": {
				Type:        schema.TypeSet,
				Description: "Virtual Networks attached to the VM.",
				Optional:    true,
				Computed:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"network_speed_profile": {
				Type:        schema.TypeString,
				Description: "Permalink of a Network Speed Profile.",
				Computed:    true,
				Optional:    true,
			},
			"network_interfaces": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Network interface details for the " +
					"virtual machine.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The ID of the network interface.",
						},
						"network_id": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "The ID of the network the " +
								"interface is attached to.",
						},
						"virtual_network_id": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "The ID of the virtual " +
								"network the interface is attached to.",
						},
						"mac_address": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "The MAC address of the " +
								"interface.",
						},
						"ip_addresses": {
							Type:     schema.TypeSet,
							Computed: true,
							Description: "The IP addresses " +
								"allocated to the interface.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"group_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func resourceVirtualMachineCustomizeDiff(
	_ context.Context,
	d *schema.ResourceDiff,
	_ interface{},
) error {
	if d.HasChange("ip_address_ids") {
		err := d.SetNewComputed("ip_addresses")
		if err != nil {
			return err
		}
	}

	if d.HasChange("hostname") {
		err := d.SetNewComputed("fqdn")
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:funlen,gocyclo
func resourceVirtualMachineCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	dcSpec := &buildspec.DataCenter{}
	switch {
	case m.DataCenterRef.ID != "":
		dcSpec.ID = m.DataCenterRef.ID
	default:
		dcSpec.Permalink = m.DataCenterRef.Permalink
	}

	spec := &buildspec.VirtualMachineSpec{
		DataCenter: dcSpec,
		Hostname:   m.UseOrGenerateHostname(d.Get("hostname").(string)),
		AuthorizedKeys: &buildspec.AuthorizedKeys{
			AllSSHKeys: true,
			AllUsers:   true,
		},
	}

	if name, ok := d.GetOk("name"); ok {
		spec.Name = name.(string)
	}

	if description, ok := d.GetOk("description"); ok {
		spec.Description = description.(string)
	}

	if rawTags, ok := d.GetOk("tags"); ok {
		spec.Tags = schemaSetToSlice[string](rawTags.(*schema.Set))
	}

	pkgRef := d.Get("package").(string)
	pkg := &buildspec.Package{}
	if strings.HasPrefix(pkgRef, "vmpkg_") {
		pkg.ID = pkgRef
	} else {
		pkg.Permalink = pkgRef
	}
	spec.Resources = &buildspec.Resources{Package: pkg}

	dtplRef := d.Get("disk_template").(string)
	if strings.HasPrefix(dtplRef, "dtpl_") {
		spec.DiskTemplate = &buildspec.DiskTemplate{ID: dtplRef}
	} else {
		if !strings.Contains(dtplRef, "/") {
			dtplRef = "templates/" + dtplRef
		}
		spec.DiskTemplate = &buildspec.DiskTemplate{Permalink: dtplRef}
	}

	if rawOpts, ok := d.GetOk("disk_template_options"); ok {
		for key, rawValue := range rawOpts.(map[string]interface{}) {
			spec.DiskTemplate.Options = append(
				spec.DiskTemplate.Options,
				&buildspec.DiskTemplateOption{
					Key:   key,
					Value: rawValue.(string),
				},
			)
		}
	}

	if rawDisks, ok := d.GetOk("disk"); ok {
		for i, rawDisk := range rawDisks.([]interface{}) {
			disk := rawDisk.(map[string]interface{})
			var name string
			if diskName, ok := disk["name"]; ok {
				name = diskName.(string)
			}

			if name == "" {
				if i == 0 {
					name = "System Disk"
				} else {
					name = fmt.Sprintf("Disk #%d", i+1)
				}
			}

			spec.SystemDisks = append(spec.SystemDisks, &buildspec.SystemDisk{
				Name: name,
				Size: disk["size"].(int),
			})
		}
	}

	var nsp *buildspec.NetworkSpeedProfile
	if permalink := d.Get("network_speed_profile").(string); permalink != "" {
		nsp = &buildspec.NetworkSpeedProfile{Permalink: permalink}
	}

	// ip_address_ids
	ipGroups := map[string][]*core.IPAddress{}
	ipIDs := schemaSetToSlice[string](d.Get("ip_address_ids").(*schema.Set))
	for _, ipID := range ipIDs {
		ip, _, err := m.Core.IPAddresses.GetByID(ctx, ipID)
		if err != nil {
			return diag.FromErr(err)
		}
		netID := ip.Network.ID

		ipGroups[netID] = append(ipGroups[netID], ip)
	}

	for netID, ips := range ipGroups {
		iface := &buildspec.NetworkInterface{
			Network: &buildspec.Network{ID: netID},
		}

		if nsp != nil {
			iface.SpeedProfile = nsp
		}

		for _, ip := range ips {
			iface.IPAddressAllocations = append(
				iface.IPAddressAllocations,
				&buildspec.IPAddressAllocation{
					Type:      buildspec.ExistingIPAddressAllocation,
					IPAddress: &buildspec.IPAddress{ID: ip.ID},
				},
			)
		}

		spec.NetworkInterfaces = append(spec.NetworkInterfaces, iface)
	}

	// virtual_network_ids
	virtualNetworkIDs := schemaSetToSlice[string](
		d.Get("virtual_network_ids").(*schema.Set),
	)
	for _, vnID := range virtualNetworkIDs {
		iface := &buildspec.NetworkInterface{
			VirtualNetwork: &buildspec.VirtualNetwork{ID: vnID},
		}

		if nsp != nil {
			iface.SpeedProfile = nsp
		}

		spec.NetworkInterfaces = append(spec.NetworkInterfaces, iface)
	}

	if groupID, ok := d.GetOk("group_id"); ok {
		spec.Group = &buildspec.Group{ID: groupID.(string)}
	}

	if diags.HasError() {
		return diags
	}

	if m.Logger.IsDebug() {
		xmlSpec, err := spec.XMLIndent("", "  ")
		if err == nil {
			m.Logger.Debug("Create buildspec:\n" + string(xmlSpec))
		}
	}

	initBuild, _, err := m.Core.VirtualMachineBuilds.CreateFromSpec(
		ctx, m.OrganizationRef, spec,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	buildWaiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.VirtualMachineBuildDraft),
			string(core.VirtualMachineBuildPending),
			string(core.VirtualMachineBuildBuilding),
		},
		Target: []string{
			string(core.VirtualMachineBuildComplete),
		},
		Refresh: func() (interface{}, string, error) {
			b, _, e := m.Core.VirtualMachineBuilds.GetByID(
				ctx, initBuild.ID,
			)
			if e != nil {
				return 0, "", e
			}

			return b.VirtualMachine, string(b.State), nil
		},
		Timeout:                   d.Timeout(schema.TimeoutCreate),
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	builtVM, err := buildWaiter.WaitForStateContext(ctx)
	if err != nil {
		return append(
			diags, diag.Errorf(
				"error waiting for virtual machine build to be created: %s",
				err,
			)...,
		)
	}

	vm := builtVM.(*core.VirtualMachine)

	// Only existing tags can be assigned upon creation, so if tags do not match
	// after creation, we issue a update to TagNames which will create and
	// assign tags as needed.
	if !stringsEqual(vm.TagNames, spec.Tags) {
		vm, _, err = m.Core.VirtualMachines.Update(
			ctx, vm.Ref(), &core.VirtualMachineUpdateArguments{
				TagNames: &spec.Tags,
			},
		)
		if err != nil {
			return append(diags, diag.Errorf(
				"failed to assign virtual machine tags: %s", err,
			)...)
		}
	}

	vmWaiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.VirtualMachineStopped),
			string(core.VirtualMachineAllocating),
			string(core.VirtualMachineAllocated),
			string(core.VirtualMachineStarting),
			string(core.VirtualMachineMigrating),
		},
		Target: []string{
			string(core.VirtualMachineStarted),
		},
		Refresh: func() (interface{}, string, error) {
			v, _, e := m.Core.VirtualMachines.GetByID(ctx, vm.ID)
			if e != nil {
				return 0, "", e
			}

			return v, string(v.State), nil
		},
		Timeout:                   d.Timeout(schema.TimeoutCreate),
		Delay:                     2 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	_, err = vmWaiter.WaitForStateContext(ctx)
	if err != nil {
		return append(
			diags, diag.Errorf(
				"error waiting for virtual machine to start: %s",
				err,
			)...,
		)
	}

	d.SetId(vm.ID)

	return resourceVirtualMachineRead(ctx, d, meta)
}

//nolint:funlen,gocyclo
func resourceVirtualMachineRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Id()

	vm, _, err := m.Core.VirtualMachines.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			d.SetId("")

			return diags
		} else if errors.Is(err, core.ErrObjectInTrash) {
			return append(diags, diag.FromErr(fmt.Errorf(
				"virtual machine %s: %w", id, err,
			))...)
		}

		return diag.FromErr(err)
	}

	ifaces, err := nextFetchAllVMNetworkInterfaces(ctx, m, vm.ID)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	virtualNetworkIDs := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.VirtualNetwork.IsSpecified() {
			vnet, err2 := iface.VirtualNetwork.Get()
			if err2 != nil {
				continue
			}

			if *iface.State != "attached" {
				continue
			}

			if id := *vnet.Id; id != "" {
				virtualNetworkIDs = append(virtualNetworkIDs, id)
			}
		}
	}

	// As we set the speed profile for all interfaces on a VM, we only care
	// about fetching details about any single interface.
	var nsp string
	if len(ifaces) > 0 {
		vmnet, _, err2 := m.Core.VirtualMachineNetworkInterfaces.GetByID(
			ctx, *ifaces[0].Id,
		)
		if err2 != nil {
			return append(diags, diag.FromErr(err2)...)
		}

		if vmnet.SpeedProfile != nil {
			nsp = vmnet.SpeedProfile.Permalink
		}
	}

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

	if nsp != "" {
		_ = d.Set("network_speed_profile", nsp)
	}

	if vm.Group != nil {
		_ = d.Set("group_id", vm.Group.ID)
	}

	if pkg := normalizeVirtualMachinePackage(vm.Package); pkg != "" {
		_ = d.Set("package", pkg)
	}

	err = d.Set(
		"ip_address_ids",
		stringSliceToSchemaSet(flattenIPAddressIDs(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"ip_addresses",
		stringSliceToSchemaSet(flattenIPAddresses(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"virtual_network_ids",
		stringSliceToSchemaSet(virtualNetworkIDs),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set("network_interfaces", flattenNetworkInterfaces(ifaces))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set("tags", stringSliceToSchemaSet(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	return diags
}

//nolint:gocyclo,funlen
func resourceVirtualMachineUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	vm := &core.VirtualMachine{ID: d.Id()}

	args := &core.VirtualMachineUpdateArguments{}

	if d.HasChange("name") {
		args.Name = d.Get("name").(string)
	}
	if d.HasChange("hostname") {
		args.Hostname = d.Get("hostname").(string)
	}
	if d.HasChange("description") {
		args.Description = d.Get("description").(string)
	}
	if d.HasChange("ip_address_ids") {
		targetIDs := schemaSetToSlice[string](
			d.Get("ip_address_ids").(*schema.Set),
		)

		var err error
		vm, _, err = m.Core.VirtualMachines.GetByID(ctx, vm.ID)
		if err != nil {
			return diag.FromErr(err)
		}
		vmIDs := flattenIPAddressIDs(vm.IPAddresses)

		addIDs := stringsDiff(targetIDs, vmIDs)
		removeIDs := stringsDiff(vmIDs, targetIDs)

		err = allocateIPsToVirtualMachine(ctx, m, vm, addIDs)
		if err != nil {
			return diag.FromErr(err)
		}

		for _, id := range removeIDs {
			_, err := m.Core.IPAddresses.Unallocate(
				ctx, core.IPAddressRef{ID: id},
			)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}
	if d.HasChange("virtual_network_ids") {
		targetIDs := schemaSetToSlice[string](
			d.Get("virtual_network_ids").(*schema.Set),
		)

		ifaces, err := nextFetchAllVMNetworkInterfaces(ctx, m, vm.ID)
		if err != nil {
			return diag.FromErr(err)
		}

		attachedVnetIDs := make([]string, 0, len(ifaces))
		detachedVnets := make(map[string]string)
		for _, iface := range ifaces {
			if iface.VirtualNetwork.IsSpecified() {
				vnet, err2 := iface.VirtualNetwork.Get()
				if err2 != nil {
					continue
				}

				if *iface.State == "attached" {
					attachedVnetIDs = append(attachedVnetIDs, *vnet.Id)
				} else if *iface.State == "detached" {
					detachedVnets[*vnet.Id] = *iface.Id
				}
			}
		}

		missingVnetIDs := stringsDiff(targetIDs, attachedVnetIDs)
		removeVnetIDs := stringsDiff(attachedVnetIDs, targetIDs)

		var addVnetIDs []string
		var attachIfaceIDs []string
		for _, id := range missingVnetIDs {
			if ifaceID, ok := detachedVnets[id]; ok {
				attachIfaceIDs = append(attachIfaceIDs, ifaceID)
			} else {
				addVnetIDs = append(addVnetIDs, id)
			}
		}

		err = addVirtualNetowrksToVirtualMachine(
			ctx, d, m, vm.ID, addVnetIDs,
			d.Get("network_speed_profile").(string),
		)
		if err != nil {
			return diag.FromErr(err)
		}

		err = attachVirtualMachineNetworkInterfaces(
			ctx, d, m, attachIfaceIDs,
		)
		if err != nil {
			return diag.FromErr(err)
		}

		// Find network interface IDs for the virtual networks to remove.
		var removeIfaceIDs []string
		for _, id := range removeVnetIDs {
			for _, iface := range ifaces {
				if iface.VirtualNetwork.IsSpecified() {
					vnet, err2 := iface.VirtualNetwork.Get()
					if err2 != nil {
						continue
					}

					if *vnet.Id == id {
						removeIfaceIDs = append(removeIfaceIDs, *iface.Id)
					}
				}
			}
		}

		err = removeNetworkInterfacesFromVirtualMachine(
			ctx, d, m, removeIfaceIDs,
		)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("network_speed_profile") {
		err := updateVMNetworkSpeedProfile(
			ctx, d, m, vm, d.Get("network_speed_profile").(string),
		)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("tags") {
		tags := schemaSetToSlice[string](d.Get("tags").(*schema.Set))
		args.TagNames = &tags
	}
	if d.HasChange("group_id") {
		groupID := d.Get("group_id").(string)

		if groupID == "" {
			args.Group = core.NullVirtualMachineGroupRef
		} else {
			args.Group = &core.VirtualMachineGroupRef{ID: groupID}
		}
	}

	_, _, err := m.Core.VirtualMachines.Update(ctx, vm.Ref(), args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceVirtualMachineRead(ctx, d, meta)
}

//nolint:funlen,gocyclo
func resourceVirtualMachineDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	diags := diag.Diagnostics{}

	timeout := d.Timeout(schema.TimeoutDelete)

	vm, _, err := m.Core.VirtualMachines.GetByID(ctx, d.Id())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			return diags
		} else if errors.Is(err, core.ErrObjectInTrash) {
			if m.SkipTrashObjectPurge {
				return diags
			}

			err2 := purgeTrashObjectByObjectID(ctx, m, timeout, vm.ID)
			if err2 != nil {
				diags = append(diags, diag.FromErr(fmt.Errorf(
					"failed to purge virtual machine from trash: %w",
					err2,
				))...)
			}

			return diags
		}

		return append(diags, diag.FromErr(
			fmt.Errorf("failed lookup virtual machine details: %w", err),
		)...)
	}

	stopped := false
	switch vm.State { //nolint:exhaustive
	case core.VirtualMachineStarted:
		var task *core.Task
		task, _, err = m.Core.VirtualMachines.Stop(ctx, vm.Ref())
		if err != nil && !isErrNotFoundOrInTrash(err) {
			return append(diags, diag.FromErr(
				fmt.Errorf("failed to stop virtual machine: %w", err),
			)...)
		}

		if task != nil {
			err = waitForTaskCompletion(ctx, m, timeout, task.ID)
			if err != nil && !isErrNotFoundOrInTrash(err) {
				return append(diags, diag.FromErr(
					fmt.Errorf("failed to stop virtual machine: %w", err),
				)...)
			}
		}
	case core.VirtualMachineStopping,
		core.VirtualMachineShuttingDown:
		// We only need to wait for the VM to stop.
	case core.VirtualMachineStopped:
		stopped = true
	default:
		return append(diags, diag.FromErr(
			fmt.Errorf(
				"cannot delete virtual machine in state: %s",
				string(vm.State),
			),
		)...)
	}

	if !stopped {
		_, err = waitForVirtualMachineToStop(
			ctx, m, timeout, vm.Ref(),
		)
		if err != nil && !isErrNotFoundOrInTrash(err) {
			return append(diags, diag.FromErr(
				fmt.Errorf("failed to stop virtual machine: %w", err),
			)...)
		}
	}

	// If we're leaving the VM in the trash when done, we need to change the
	// hostname to something unique, as the hostname is unique within the
	// organization, and would otherwise prevent us from creating a new VM with
	// the same hostname.
	if m.SkipTrashObjectPurge {
		vm, err = addVMUniqueHostnameSuffix(ctx, m, vm)
		if err != nil && !isErrNotFoundOrInTrash(err) {
			return append(diags, diag.FromErr(
				fmt.Errorf(
					"failed to change virtual machine hostname before "+
						"moving to trash: %w",
					err,
				),
			)...)
		}
	}

	_, _, err = m.Core.VirtualMachines.Delete(ctx, vm.Ref())
	if err != nil && !isErrNotFoundOrInTrash(err) {
		return append(diags, diag.FromErr(
			fmt.Errorf("failed to delete virtual machine: %w", err),
		)...)
	}

	err = unallocateAllVirtualMachineIPs(ctx, d, m)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	if !m.SkipTrashObjectPurge {
		err = purgeTrashObjectByObjectID(
			ctx, m, timeout, vm.ID,
		)
		if err != nil && !isErrNotFoundOrInTrash(err) {
			return append(diags, diag.FromErr(
				fmt.Errorf(
					"failed to purge virtual machine from trash: %w", err,
				),
			)...)
		}
	}

	return diags
}

// addVMUniqueHostnameSuffix appends a random string to the hostname to make it
// unique. This is specifically intended for when deleting a VM and leaving it
// in the trash, to avoid a hostname if Terraform tries to re-create the VM with
// the same hostname.
//
// We can't reliably use the VM ID, as it uses both uppercase and lowercase
// letters, and the hostname must be all lowercase.
func addVMUniqueHostnameSuffix(
	ctx context.Context,
	m *Meta,
	vm *core.VirtualMachine,
) (*core.VirtualMachine, error) {
	id, err := rands.Alphanumeric(12)
	if err != nil {
		return nil, err
	}

	hostname := vm.Hostname
	suffix := "-" + id
	if len(hostname)+len(suffix) > 63 {
		hostname = hostname[:63-len(suffix)]
	}
	hostname += suffix

	vm, _, err = m.Core.VirtualMachines.Update(
		ctx, vm.Ref(),
		&core.VirtualMachineUpdateArguments{Hostname: hostname},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to change virtual machine hostname before "+
				"moving to trash: %w",
			err,
		)
	}

	return vm, nil
}

func waitForVirtualMachineToStop(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	vmRef core.VirtualMachineRef,
) (*core.VirtualMachine, error) {
	waiter := &retry.StateChangeConf{
		Pending: []string{
			string(core.VirtualMachineStarted),
			string(core.VirtualMachineStopping),
			string(core.VirtualMachineShuttingDown),
		},
		Target: []string{
			string(core.VirtualMachineStopped),
		},
		Refresh: func() (interface{}, string, error) {
			vm, _, e := m.Core.VirtualMachines.Get(ctx, vmRef)
			if e != nil {
				return vm, "", e
			}

			return vm, string(vm.State), nil
		},
		Timeout:                   timeout,
		Delay:                     1 * time.Second,
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 1,
	}

	rawVM, err := waiter.WaitForStateContext(ctx)

	return rawVM.(*core.VirtualMachine), err
}

func normalizeVirtualMachinePackage(
	pkg *core.VirtualMachinePackage,
) string {
	if pkg == nil {
		return ""
	}

	if pkg.Permalink != "" {
		return pkg.Permalink
	}

	return pkg.ID
}

func flattenIPAddressIDs(ips []*core.IPAddress) []string {
	ids := make([]string, 0, len(ips))
	for _, ip := range ips {
		ids = append(ids, ip.ID)
	}

	return ids
}

func flattenIPAddresses(ips []*core.IPAddress) []string {
	addresses := make([]string, 0, len(ips))
	for _, ip := range ips {
		addresses = append(addresses, ip.Address)
	}

	return addresses
}

func flattenNetworkInterfaces(
	ifaces []*corenext.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface,
) []map[string]any {
	vmnis := make([]map[string]any, 0, len(ifaces))
	for _, iface := range ifaces {
		vmni := map[string]any{"id": iface.Id}

		if iface.Network.IsSpecified() {
			network, err2 := iface.Network.Get()
			if err2 == nil && *network.Id != "" {
				vmni["network_id"] = network.Id
			}
		}
		if iface.VirtualNetwork.IsSpecified() {
			vnet, err2 := iface.VirtualNetwork.Get()
			if err2 == nil && *vnet.Id != "" {
				vmni["virtual_network_id"] = vnet.Id
			}
		}

		if iface.MacAddress != nil {
			vmni["mac_address"] = *iface.MacAddress
		}

		if iface.IpAddresses != nil && len(*iface.IpAddresses) > 0 {
			ipAddrs := make([]string, 0, len(*iface.IpAddresses))
			for _, ip := range *iface.IpAddresses {
				if ip.Address != nil {
					ipAddrs = append(ipAddrs, *ip.Address)
				}
			}
			vmni["ip_addresses"] = stringSliceToSchemaSet(ipAddrs)
		}

		vmnis = append(vmnis, vmni)
	}
	return vmnis
}

func unallocateAllVirtualMachineIPs(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
) error {
	ipIDs := schemaSetToSlice[string](d.Get("ip_address_ids").(*schema.Set))

	for _, ipID := range ipIDs {
		ip := &core.IPAddress{ID: ipID}
		_, err := m.Core.IPAddresses.Unallocate(ctx, ip.Ref())
		if err != nil && !errors.Is(err, katapult.ErrNotFound) {
			return fmt.Errorf(
				"failed to unallocate IP %s from virtual machine %s: %w",
				ipID, d.Id(), err,
			)
		}
	}

	return nil
}

func allocateIPsToVirtualMachine(
	ctx context.Context,
	m *Meta,
	vm *core.VirtualMachine,
	ipIDs []string,
) error {
	if len(ipIDs) == 0 {
		return nil
	}

	vmnets, err := fetchAllVMNetworkInterfaces(ctx, m, vm)
	if err != nil {
		return err
	}

	for _, ipID := range ipIDs {
		ip, _, err2 := m.Core.IPAddresses.GetByID(ctx, ipID)
		if err2 != nil {
			return err2
		}

		if ip.Network == nil {
			return fmt.Errorf("could not determine network of IP ID: %s", ipID)
		}

		var vmnet *core.VirtualMachineNetworkInterface
		for _, ni := range vmnets {
			if ni.Network != nil && ni.Network.ID == ip.Network.ID {
				vmnet = ni
			}
		}

		if vmnet == nil {
			return fmt.Errorf(
				"no usable network interface found for IP ID: %s", ipID,
			)
		}

		_, _, err2 = m.Core.VirtualMachineNetworkInterfaces.AllocateIP(
			ctx, vmnet.Ref(), ip.Ref(),
		)
		if err2 != nil {
			return err2
		}
	}

	return err
}

func fetchAllVMNetworkInterfaces(
	ctx context.Context,
	m *Meta,
	vm *core.VirtualMachine,
) ([]*core.VirtualMachineNetworkInterface, error) {
	var vmnets []*core.VirtualMachineNetworkInterface
	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		pageResult, r, err := m.Core.VirtualMachineNetworkInterfaces.List(
			ctx, vm.Ref(), &core.ListOptions{
				Page: pageNum,
			},
		)
		if err != nil {
			return nil, err
		}

		totalPages = r.Pagination.TotalPages
		vmnets = append(vmnets, pageResult...)
	}

	return vmnets, nil
}

//nolint:lll
func nextFetchAllVMNetworkInterfaces(
	ctx context.Context,
	m *Meta,
	vmID string,
) ([]*corenext.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface, error) {
	results := make(map[string]*corenext.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface)

	totalPages := 2
	for page := 1; page <= totalPages; page++ {
		resp, err := m.CoreNext.GetVirtualMachineNetworkInterfacesWithResponse(
			ctx, &corenext.GetVirtualMachineNetworkInterfacesParams{
				VirtualMachineId: &vmID,
			},
		)
		if err != nil {
			if resp != nil {
				return nil, genericAPIError(err, resp.Body)
			}

			return nil, err
		}

		if resp.JSON200 == nil {
			return nil, fmt.Errorf("unexpected empty response")
		}

		body := resp.JSON200
		if body.Pagination.Total.IsSpecified() {
			n, _ := body.Pagination.Total.Get()
			totalPages = n
		}

		for _, iface := range body.VirtualMachineNetworkInterfaces {
			vmni, errGet := getVirtualMachineNetworkInterface(
				ctx, m, *iface.Id,
			)
			if errGet != nil {
				return nil, errGet
			}

			if id := *vmni.Id; id != "" {
				results[id] = vmni
			}
		}
	}

	ifaces := make(
		[]*corenext.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface,
		0,
		len(results),
	)
	for _, iface := range results {
		ifaces = append(ifaces, iface)
	}

	sort.Slice(ifaces, func(i, j int) bool {
		return *ifaces[i].Id < *ifaces[j].Id
	})

	return ifaces, nil
}

func addVirtualNetowrksToVirtualMachine(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	vmID string,
	vnetIDs []string,
	speedProfile string,
) error {
	for _, vnetID := range vnetIDs {
		err := addVirtualNetowrkToVirtualMachine(
			ctx, d, m, vmID, vnetID, speedProfile,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:lll
func addVirtualNetowrkToVirtualMachine(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	vmID string,
	vnetID string,
	speedProfile string,
) error {
	createResp, err := m.CoreNext.PostVirtualMachineNetworkInterfacesWithResponse(
		ctx,
		corenext.PostVirtualMachineNetworkInterfacesJSONRequestBody{
			VirtualMachine: corenext.VirtualMachineLookup{Id: &vmID},
			VirtualNetwork: &corenext.VirtualNetworkLookup{Id: &vnetID},
			SpeedProfile: corenext.NetworkSpeedProfileLookup{
				Permalink: &speedProfile,
			},
		},
	)
	if err != nil {
		if createResp != nil {
			return genericAPIError(err, createResp.Body)
		}

		return err
	}
	if createResp.JSON200 == nil {
		return fmt.Errorf("unexpected empty response")
	}

	iface := createResp.JSON200.VirtualMachineNetworkInterface

	err = attachVirtualMachineNetworkInterface(
		ctx, m, *iface.Id, d.Timeout(schema.TimeoutUpdate),
	)
	if err != nil {
		return err
	}

	return nil
}

func removeNetworkInterfacesFromVirtualMachine(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	ifaceIDs []string,
) error {
	for _, vnetID := range ifaceIDs {
		err := removeNetworkInterfaceFromVirtualMachine(ctx, d, m, vnetID)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:lll
func removeNetworkInterfaceFromVirtualMachine(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	ifaceID string,
) error {
	iface, err := getVirtualMachineNetworkInterface(ctx, m, ifaceID)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			return nil
		}

		return err
	}

	state := *iface.State
	if state == "attached" {
		err = detatchVirtualMachineNetworkInterface(
			ctx, m, ifaceID, d.Timeout(schema.TimeoutUpdate),
		)
		if err != nil {
			return err
		}
	}

	delResp, err := m.CoreNext.DeleteVirtualMachineNetworkInterfaceWithResponse(
		ctx,
		corenext.DeleteVirtualMachineNetworkInterfaceJSONRequestBody{
			VirtualMachineNetworkInterface: corenext.VirtualMachineNetworkInterfaceLookup{
				Id: &ifaceID,
			},
		},
	)
	if err != nil {
		if delResp != nil {
			if delResp.StatusCode() == http.StatusNotFound {
				return nil
			}

			return genericAPIError(err, delResp.Body)
		}

		return err
	}
	if delResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected empty response")
	}

	return nil
}

func getVirtualMachineNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
) (*corenext.GetVMNIVMNI200ResponseVirtualMachineNetworkInterface, error) {
	getResp, err := m.CoreNext.GetVMNIVMNIWithResponse(ctx,
		&corenext.GetVMNIVMNIParams{
			VirtualMachineNetworkInterfaceId: &ifaceID,
		},
	)
	if err != nil {
		if getResp != nil {
			if getResp.StatusCode() == http.StatusNotFound {
				return nil, katapult.ErrNotFound
			}

			return nil, genericAPIError(err, getResp.Body)
		}

		return nil, err
	}

	if getResp.JSON200 == nil {
		return nil, fmt.Errorf("unexpected empty response")
	}

	return &getResp.JSON200.VirtualMachineNetworkInterface, nil
}

func attachVirtualMachineNetworkInterfaces(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	ifaceIDs []string,
) error {
	for _, ifaceID := range ifaceIDs {
		err := attachVirtualMachineNetworkInterface(
			ctx, m, ifaceID, d.Timeout(schema.TimeoutUpdate),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

//nolint:lll
func attachVirtualMachineNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
	timeout time.Duration,
) error {
	attachResp, err := m.CoreNext.PostVirtualMachineNetworkInterfaceAttachWithResponse(
		ctx,
		corenext.PostVirtualMachineNetworkInterfaceAttachJSONRequestBody{
			VirtualMachineNetworkInterface: corenext.VirtualMachineNetworkInterfaceLookup{
				Id: &ifaceID,
			},
		},
	)
	if err != nil {
		if attachResp != nil {
			return genericAPIError(err, attachResp.Body)
		}

		return err
	}

	if attachResp.JSON200 == nil || attachResp.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response")
	}

	taskID := *attachResp.JSON200.Task.Id

	err = waitForTaskCompletion(ctx, m, timeout, taskID)
	if err != nil {
		return err
	}

	return nil
}

//nolint:lll
func detatchVirtualMachineNetworkInterface(
	ctx context.Context,
	m *Meta,
	ifaceID string,
	timeout time.Duration,
) error {
	detatchResp, err := m.CoreNext.PostVirtualMachineNetworkInterfaceDetachWithResponse(
		ctx,
		corenext.PostVirtualMachineNetworkInterfaceDetachJSONRequestBody{
			VirtualMachineNetworkInterface: corenext.VirtualMachineNetworkInterfaceLookup{
				Id: &ifaceID,
			},
		},
	)
	if err != nil {
		if detatchResp != nil {
			if detatchResp.StatusCode() == http.StatusNotFound {
				return nil
			}

			apiErr := parseGenericAPIError(detatchResp.Body)
			if apiErr == nil {
				return err
			}

			if apiErr.Code != "virtual_machine_network_interface_not_attached" {
				return apiErr
			}
		}

		return err
	}

	// Detach task is returned if the interface was attached, so we need to
	// wait for the detatch task to complete.
	if detatchResp.JSON200 == nil || detatchResp.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response")
	}

	taskID := *detatchResp.JSON200.Task.Id

	err = waitForTaskCompletion(ctx, m, timeout, taskID)
	if err != nil {
		return err
	}

	return nil
}

func updateVMNetworkSpeedProfile(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
	vm *core.VirtualMachine,
	speedProfilePermalink string,
) error {
	if speedProfilePermalink == "" {
		return nil
	}

	vmnets, err := fetchAllVMNetworkInterfaces(ctx, m, vm)
	if err != nil {
		return err
	}

	for _, vmnet := range vmnets {
		task, _, err := m.Core.VirtualMachineNetworkInterfaces.
			UpdateSpeedProfile(
				ctx, vmnet.Ref(), core.NetworkSpeedProfileRef{
					Permalink: speedProfilePermalink,
				},
			)
		if err != nil {
			if errors.Is(err, core.ErrSpeedProfileAlreadyAssigned) {
				continue
			}

			return err
		}

		err = waitForTaskCompletion(
			ctx, m, d.Timeout(schema.TimeoutUpdate), task.ID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
