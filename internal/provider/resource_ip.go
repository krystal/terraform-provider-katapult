package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult/pkg/katapult"
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
			"network_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"version": {
				Type:         schema.TypeInt,
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
				Optional:     true,
				RequiredWith: []string{"vip"},
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
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)

	org := meta.Organization()
	dc := meta.DataCenter()

	var network *katapult.Network
	if rawNet, ok := d.GetOk("network_id"); ok {
		network = &katapult.Network{ID: rawNet.(string)}
	} else {
		var err error
		network, err = defaultNetworkForDataCenter(ctx, meta, dc)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	args := &katapult.IPAddressCreateArguments{
		Network: network,
		Version: unflattenIPVersion(d.Get("version").(int)),
	}

	if vip := d.Get("vip").(bool); vip {
		args.VIP = &vip
		args.Label = d.Get("label").(string)
	}

	ip, _, err := meta.Client.IPAddresses.Create(ctx, org, args)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(ip.ID)

	return resourceIPRead(ctx, d, m)
}

func resourceIPRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	var diags diag.Diagnostics

	ip, resp, err := meta.Client.IPAddresses.GetByID(ctx, d.Id())
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
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
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)

	ip := &katapult.IPAddress{ID: d.Id()}
	args := &katapult.IPAddressUpdateArguments{}

	if d.HasChange("vip") {
		vip := d.Get("vip").(bool)
		args.VIP = &vip
	}

	if d.HasChange("label") {
		args.Label = d.Get("label").(string)
	}

	_, _, err := meta.Client.IPAddresses.Update(ctx, ip, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceIPRead(ctx, d, m)
}

func resourceIPDelete(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)

	ip := &katapult.IPAddress{ID: d.Id()}

	_, err := meta.Client.IPAddresses.Delete(ctx, ip)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}

func unflattenIPVersion(ver int) katapult.IPVersion {
	switch ver {
	case 6:
		return katapult.IPv6
	default:
		return katapult.IPv4
	}
}

func flattenIPVersion(address string) int {
	if strings.Count(address, ":") < 2 {
		return 4
	}

	return 6
}
