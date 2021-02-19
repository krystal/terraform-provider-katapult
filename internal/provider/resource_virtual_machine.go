package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult/pkg/buildspec"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func resourceVirtualMachine() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceVirtualMachineCreate,
		ReadContext:   resourceVirtualMachineRead,
		UpdateContext: resourceVirtualMachineUpdate,
		DeleteContext: resourceVirtualMachineDelete,
		CustomizeDiff: resourceVirtualMachineCustomizeDiff,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(time.Minute * 10),
			Delete: schema.DefaultTimeout(time.Minute * 5),
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
				Required:     true,
				ForceNew:     true, // TODO: Add support for changing package
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"disk_template": {
				Type:         schema.TypeString,
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
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
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
			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceVirtualMachineCustomizeDiff(
	ctx context.Context,
	d *schema.ResourceDiff,
	m interface{},
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
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	var diags diag.Diagnostics

	dcSpec := &buildspec.DataCenter{}
	switch {
	case meta.DataCenterRef().ID != "":
		dcSpec.ID = meta.DataCenterRef().ID
	default:
		dcSpec.Permalink = meta.DataCenterRef().Permalink
	}

	spec := &buildspec.VirtualMachineSpec{
		DataCenter: dcSpec,
		Hostname:   meta.UseOrGenerateHostname(d.Get("hostname").(string)),
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

	ipGroups := map[string][]*katapult.IPAddress{}
	for _, rawIP := range d.Get("ip_address_ids").(*schema.Set).List() {
		ip, _, err := meta.Client.IPAddresses.GetByID(ctx, rawIP.(string))
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

	if diags.HasError() {
		return diags
	}

	initBuild, _, err := meta.Client.VirtualMachineBuilds.CreateFromSpec(
		ctx, meta.OrganizationRef(), spec,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	buildWaiter := &resource.StateChangeConf{
		Pending: []string{
			string(katapult.VirtualMachineBuildDraft),
			string(katapult.VirtualMachineBuildPending),
			string(katapult.VirtualMachineBuildBuilding),
		},
		Target: []string{
			string(katapult.VirtualMachineBuildComplete),
		},
		Refresh: func() (interface{}, string, error) {
			b, _, e := meta.Client.VirtualMachineBuilds.GetByID(
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

	vm := builtVM.(*katapult.VirtualMachine)

	// Only existing tags can be assigned upon creation, so if tags do not match
	// after creation, we issue a update to TagNames which will create and
	// assign tags as needed.
	if !stringsEqual(vm.TagNames, spec.Tags) {
		vm, _, err = meta.Client.VirtualMachines.Update(
			ctx, vm, &katapult.VirtualMachineUpdateArguments{
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
			string(katapult.VirtualMachineStopped),
			string(katapult.VirtualMachineStarting),
			string(katapult.VirtualMachineMigrating),
		},
		Target: []string{
			string(katapult.VirtualMachineStarted),
		},
		Refresh: func() (interface{}, string, error) {
			v, _, e := meta.Client.VirtualMachines.GetByID(ctx, vm.ID)
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

	return resourceVirtualMachineRead(ctx, d, m)
}

func resourceVirtualMachineRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	c := m.(*Meta).Client
	var diags diag.Diagnostics

	id := d.Id()

	vm, resp, err := c.VirtualMachines.GetByID(ctx, id)
	if err != nil {
		if resp != nil {
			if resp.Response != nil && resp.StatusCode == 404 {
				d.SetId("")

				return diags
			} else if resp.Error != nil &&
				resp.Error.Code == "object_in_trash" {
				return append(diags, diag.FromErr(fmt.Errorf(
					"virtual machine %s: %w", id, err,
				))...)
			}
		}

		return diag.FromErr(err)
	}

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

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

	err = d.Set("tags", flattenTagNames(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	return diags
}

func resourceVirtualMachineUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)

	vm := &katapult.VirtualMachine{ID: d.Id()}

	args := &katapult.VirtualMachineUpdateArguments{}

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
		vm, _, err = meta.Client.VirtualMachines.GetByID(ctx, vm.ID)
		if err != nil {
			return diag.FromErr(err)
		}
		vmIDs := flattenIPAddressIDs(vm.IPAddresses)

		addIDs := stringsDiff(targetIDs, vmIDs)
		removeIDs := stringsDiff(vmIDs, targetIDs)

		for _, id := range addIDs {
			err := allocateIPToVirtualMachine(ctx, meta, vm, id)
			if err != nil {
				return diag.FromErr(err)
			}
		}

		for _, id := range removeIDs {
			_, err := meta.Client.IPAddresses.Unallocate(
				ctx, &katapult.IPAddress{ID: id},
			)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}
	if d.HasChange("tags") {
		tags := []string{}
		for _, tag := range d.Get("tags").(*schema.Set).List() {
			tags = append(tags, tag.(string))
		}
		args.TagNames = &tags
	}

	_, _, err := meta.Client.VirtualMachines.Update(ctx, vm, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceVirtualMachineRead(ctx, d, m)
}

func resourceVirtualMachineDelete( //nolint:funlen
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	diags := diag.Diagnostics{}

	vm, resp, err := meta.Client.VirtualMachines.GetByID(ctx, d.Id())
	if err != nil {
		if resp != nil {
			if resp.Response != nil && resp.StatusCode == 404 {
				return diags
			} else if resp.Error != nil &&
				resp.Error.Code == "object_in_trash" {
				err2 := purgeTrashObjectByObjectID(
					ctx, meta, d.Timeout(schema.TimeoutDelete), vm.ID,
				)
				if err2 != nil {
					diags = append(diags, diag.FromErr(fmt.Errorf(
						"failed to purge virtual machine from trash: %w",
						err2,
					))...)
				}

				return diags
			}
		}

		return append(diags, diag.FromErr(
			fmt.Errorf("failed lookup virtual machine details: %w", err),
		)...)
	}

	switch vm.State {
	case katapult.VirtualMachineStarted:
		_, _, err2 := meta.Client.VirtualMachines.Stop(ctx, vm)
		if err2 != nil {
			return append(diags, diag.FromErr(
				fmt.Errorf("failed to stop virtual machine: %w", err2),
			)...)
		}
	case katapult.VirtualMachineStopped,
		katapult.VirtualMachineStopping,
		katapult.VirtualMachineShuttingDown:
		// no action needed
	default:
		return append(diags, diag.FromErr(
			fmt.Errorf(
				"cannot delete virtual machine in state: %s",
				string(vm.State),
			),
		)...)
	}

	if vm.State != katapult.VirtualMachineStopped {
		vmWaiter := &resource.StateChangeConf{
			Pending: []string{
				string(katapult.VirtualMachineStarted),
				string(katapult.VirtualMachineStopping),
				string(katapult.VirtualMachineShuttingDown),
			},
			Target: []string{
				string(katapult.VirtualMachineStopped),
			},
			Refresh: func() (interface{}, string, error) {
				v, _, err2 := meta.Client.VirtualMachines.GetByID(ctx, vm.ID)
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
	}

	trash, _, err := meta.Client.VirtualMachines.Delete(ctx, vm)
	if err != nil {
		return append(diags, diag.FromErr(
			fmt.Errorf("failed to delete virtual machine: %w", err),
		)...)
	}

	err = unallocateAllVirtualMachineIPs(ctx, d, meta)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	err = purgeTrashObject(ctx, meta, d.Timeout(schema.TimeoutDelete), trash)
	if err != nil {
		return append(diags, diag.FromErr(
			fmt.Errorf("failed to purge virtual machine from trash: %w", err),
		)...)
	}

	return diags
}

func normalizeVirtualMachinePackage(
	pkg *katapult.VirtualMachinePackage,
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

func flattenIPAddressIDs(ips []*katapult.IPAddress) []string {
	var ids []string
	for _, ip := range ips {
		ids = append(ids, ip.ID)
	}

	return ids
}

func flattenIPAddresses(ips []*katapult.IPAddress) []string {
	var addresses []string
	for _, ip := range ips {
		addresses = append(addresses, ip.Address)
	}

	return addresses
}

func unallocateAllVirtualMachineIPs(
	ctx context.Context,
	d *schema.ResourceData,
	meta *Meta,
) error {
	ipIDs := d.Get("ip_address_ids").(*schema.Set).List()
	for _, ipID := range ipIDs {
		ip := &katapult.IPAddress{ID: ipID.(string)}
		_, err := meta.Client.IPAddresses.Unallocate(ctx, ip)
		if err != nil {
			return fmt.Errorf(
				"failed to unallocate IP %s from virtual machine %s: %w",
				ipID, d.Id(), err,
			)
		}
	}

	return nil
}

func allocateIPToVirtualMachine(
	ctx context.Context,
	meta *Meta,
	vm *katapult.VirtualMachine,
	ipID string,
) error {
	ip, _, err := meta.Client.IPAddresses.GetByID(ctx, ipID)
	if err != nil {
		return err
	}

	vmnet, err := fetchVMNetworkInterface(ctx, meta, vm, ip.Network)
	if err != nil {
		return err
	}

	_, _, err = meta.Client.VirtualMachineNetworkInterfaces.AllocateIP(
		ctx, vmnet, ip,
	)

	return err
}

func fetchVMNetworkInterface(
	ctx context.Context,
	meta *Meta,
	vm *katapult.VirtualMachine,
	net *katapult.Network,
) (*katapult.VirtualMachineNetworkInterface, error) {
	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		nis, r, err := meta.Client.VirtualMachineNetworkInterfaces.List(
			ctx, vm, &katapult.ListOptions{
				Page: pageNum,
			},
		)
		if err != nil {
			return nil, err
		}

		totalPages = r.Pagination.TotalPages

		for _, n := range nis {
			if n.Network == nil {
				continue
			}

			if (net.ID != "" && n.Network.ID == net.ID) ||
				(net.Permalink != "" && n.Network.Permalink == net.Permalink) {
				return n, nil
			}
		}
	}

	return nil, fmt.Errorf("no network interface found")
}
