package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/buildspec"
	"github.com/krystal/go-katapult/core"
)

func resourceVirtualMachine() *schema.Resource {
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
			"network_speed_profile": {
				Type:        schema.TypeString,
				Description: "Permalink of a Network Speed Profile.",
				Computed:    true,
				Optional:    true,
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
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
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
		for _, tag := range rawTags.(*schema.Set).List() {
			spec.Tags = append(spec.Tags, tag.(string))
		}
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

	ipGroups := map[string][]*core.IPAddress{}
	for _, rawIP := range d.Get("ip_address_ids").(*schema.Set).List() {
		ip, _, err := m.Core.IPAddresses.GetByID(ctx, rawIP.(string))
		if err != nil {
			return diag.FromErr(err)
		}
		netID := ip.Network.ID

		ipGroups[netID] = append(ipGroups[netID], ip)
	}

	var nsp *buildspec.NetworkSpeedProfile
	if permalink := d.Get("network_speed_profile").(string); permalink != "" {
		nsp = &buildspec.NetworkSpeedProfile{Permalink: permalink}
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

	buildWaiter := &resource.StateChangeConf{
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

	vmWaiter := &resource.StateChangeConf{
		Pending: []string{
			string(core.VirtualMachineStopped),
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

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

	if vm.Group != nil {
		_ = d.Set("group_id", vm.Group.ID)
	}

	if pkg := normalizeVirtualMachinePackage(vm.Package); pkg != "" {
		_ = d.Set("package", pkg)
	}

	err = d.Set(
		"ip_address_ids",
		newSchemaStringSet(flattenIPAddressIDs(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"ip_addresses",
		newSchemaStringSet(flattenIPAddresses(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	// As we set the speed profile for all interfaces on a VM, we only care
	// about fetching details about any single interface.
	vmnets, _, err := m.Core.VirtualMachineNetworkInterfaces.List(
		ctx, vm.Ref(), &core.ListOptions{Page: 1, PerPage: 1},
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	} else if len(vmnets) > 0 {
		vmnet, _, err2 := m.Core.VirtualMachineNetworkInterfaces.Get(
			ctx, vmnets[0].Ref(),
		)
		if err2 != nil {
			diags = append(diags, diag.FromErr(err2)...)
		} else if vmnet.SpeedProfile != nil {
			_ = d.Set("network_speed_profile", vmnet.SpeedProfile.Permalink)
		}
	}

	err = d.Set("tags", flattenTagNames(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	return diags
}

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
		var targetIDs []string
		for _, rawID := range d.Get("ip_address_ids").(*schema.Set).List() {
			targetIDs = append(targetIDs, rawID.(string))
		}

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
	if d.HasChange("network_speed_profile") {
		err := updateVMNetworkSpeedProfile(
			ctx, d, m, vm, d.Get("network_speed_profile").(string),
		)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("tags") {
		tags := []string{}
		for _, tag := range d.Get("tags").(*schema.Set).List() {
			tags = append(tags, tag.(string))
		}
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

func resourceVirtualMachineDelete( //nolint:funlen
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	diags := diag.Diagnostics{}

	vm, _, err := m.Core.VirtualMachines.GetByID(ctx, d.Id())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			return diags
		} else if errors.Is(err, core.ErrObjectInTrash) {
			err2 := purgeTrashObjectByObjectID(
				ctx, m, d.Timeout(schema.TimeoutDelete), vm.ID,
			)
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

	switch vm.State {
	case core.VirtualMachineStarted:
		err2 := stopVirtualMachine(ctx, m, d.Timeout(schema.TimeoutDelete), vm)
		if err2 != nil {
			return append(diags, diag.FromErr(
				fmt.Errorf("failed to stop virtual machine: %w", err2),
			)...)
		}
	case core.VirtualMachineStopping,
		core.VirtualMachineShuttingDown:
		vmWaiter := &resource.StateChangeConf{
			Pending: []string{
				string(core.VirtualMachineStarted),
				string(core.VirtualMachineStopping),
				string(core.VirtualMachineShuttingDown),
			},
			Target: []string{
				string(core.VirtualMachineStopped),
			},
			Refresh: func() (interface{}, string, error) {
				v, _, err2 := m.Core.VirtualMachines.GetByID(ctx, vm.ID)
				if err2 != nil {
					return 0, "", err2
				}

				return v, string(v.State), nil
			},
			Timeout:                   d.Timeout(schema.TimeoutDelete),
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		_, err = vmWaiter.WaitForStateContext(ctx)
		if err != nil {
			return append(diags, diag.FromErr(
				fmt.Errorf("failed to stop virtual machine: %w", err),
			)...)
		}
	case core.VirtualMachineStopped:
		// no action needed
	default:
		return append(diags, diag.FromErr(
			fmt.Errorf(
				"cannot delete virtual machine in state: %s",
				string(vm.State),
			),
		)...)
	}

	trash, _, err := m.Core.VirtualMachines.Delete(ctx, vm.Ref())
	if err != nil {
		return append(diags, diag.FromErr(
			fmt.Errorf("failed to delete virtual machine: %w", err),
		)...)
	}

	err = unallocateAllVirtualMachineIPs(ctx, d, m)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	err = purgeTrashObject(ctx, m, d.Timeout(schema.TimeoutDelete), trash)
	if err != nil {
		return append(diags, diag.FromErr(
			fmt.Errorf("failed to purge virtual machine from trash: %w", err),
		)...)
	}

	return diags
}

func stopVirtualMachine(
	ctx context.Context,
	m *Meta,
	timeout time.Duration,
	vm *core.VirtualMachine,
) error {
	task, _, err := m.Core.VirtualMachines.Stop(ctx, vm.Ref())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			return nil
		}

		return err
	}

	_, err = waitForTaskCompletion(ctx, m, timeout, task)

	return err
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

func flattenTagNames(names []string) *schema.Set {
	var v []interface{}
	for _, name := range names {
		v = append(v, name)
	}

	return schema.NewSet(stringHash, v)
}

func flattenIPAddressIDs(ips []*core.IPAddress) []string {
	var ids []string
	for _, ip := range ips {
		ids = append(ids, ip.ID)
	}

	return ids
}

func flattenIPAddresses(ips []*core.IPAddress) []string {
	var addresses []string
	for _, ip := range ips {
		addresses = append(addresses, ip.Address)
	}

	return addresses
}

func unallocateAllVirtualMachineIPs(
	ctx context.Context,
	d *schema.ResourceData,
	m *Meta,
) error {
	ipIDs := d.Get("ip_address_ids").(*schema.Set).List()
	for _, ipID := range ipIDs {
		ip := &core.IPAddress{ID: ipID.(string)}
		_, err := m.Core.IPAddresses.Unallocate(ctx, ip.Ref())
		if err != nil {
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

		_, err = waitForTaskCompletion(
			ctx, m, d.Timeout(schema.TimeoutUpdate), task,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
