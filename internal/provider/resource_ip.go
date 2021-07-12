package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

func resourceIP() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceIPCreate,
		ReadContext:   resourceIPRead,
		UpdateContext: resourceIPUpdate,
		DeleteContext: resourceIPDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"network_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"version": {
				Type:         schema.TypeInt,
				Description:  "IPv4 or IPv6.",
				Optional:     true,
				ForceNew:     true,
				Default:      4,
				ValidateFunc: validation.IntInSlice([]int{4, 6}),
			},
			"address": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"address_with_mask": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"reverse_dns": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"vip": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"label": {
				Type:         schema.TypeString,
				Description:  "VIP label. Required when **vip** is `true`.",
				Optional:     true,
				RequiredWith: []string{"vip"},
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"allocation_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"allocation_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceIPCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	var network *core.Network
	if rawNet, ok := d.GetOk("network_id"); ok {
		network = &core.Network{ID: rawNet.(string)}
	} else {
		var err error
		network, _, err = m.Core.DataCenters.DefaultNetwork(
			ctx, m.DataCenterRef,
		)

		if err != nil {
			return diag.FromErr(err)
		}
	}

	args := &core.IPAddressCreateArguments{
		Network: network.Ref(),
		Version: unflattenIPVersion(d.Get("version").(int)),
	}

	if vip := d.Get("vip").(bool); vip {
		args.VIP = &vip
		args.Label = d.Get("label").(string)
	}

	ip, _, err := m.Core.IPAddresses.Create(
		ctx, m.OrganizationRef, args,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(ip.ID)

	return resourceIPRead(ctx, d, meta)
}

func resourceIPRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	ip, _, err := m.Core.IPAddresses.GetByID(ctx, d.Id())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			d.SetId("")

			return diags
		}

		return diag.FromErr(err)
	}

	if ip.Network != nil {
		_ = d.Set("network_id", ip.Network.ID)
	}
	_ = d.Set("address", ip.Address)
	_ = d.Set("address_with_mask", ip.AddressWithMask)
	_ = d.Set("reverse_dns", ip.ReverseDNS)
	_ = d.Set("version", flattenIPVersion(ip.Address))
	_ = d.Set("vip", ip.VIP)
	_ = d.Set("label", ip.Label)
	_ = d.Set("allocation_type", ip.AllocationType)
	_ = d.Set("allocation_id", ip.AllocationID)

	return diags
}

func resourceIPUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	ipRef := core.IPAddressRef{ID: d.Id()}
	args := &core.IPAddressUpdateArguments{}

	if d.HasChange("vip") {
		vip := d.Get("vip").(bool)
		args.VIP = &vip
	}

	if d.HasChange("label") {
		args.Label = d.Get("label").(string)
	}

	_, _, err := m.Core.IPAddresses.Update(ctx, ipRef, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceIPRead(ctx, d, meta)
}

func resourceIPDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	ipRef := core.IPAddressRef{ID: d.Id()}
	_, err := m.Core.IPAddresses.Delete(ctx, ipRef)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}

func unflattenIPVersion(ver int) core.IPVersion {
	switch ver {
	case 6:
		return core.IPv6
	default:
		return core.IPv4
	}
}

func flattenIPVersion(address string) int {
	if strings.Count(address, ":") < 2 {
		return 4
	}

	return 6
}
