package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func resourceLoadBalancer() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceLoadBalancerCreate,
		ReadContext:   resourceLoadBalancerRead,
		UpdateContext: resourceLoadBalancerUpdate,
		DeleteContext: resourceLoadBalancerDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
			},
			"resource_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"virtual_machine": {
				Type:          schema.TypeList,
				Optional:      true,
				ConflictsWith: []string{"tag", "virtual_machine_group"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"virtual_machine_group": {
				Type:          schema.TypeList,
				Optional:      true,
				ConflictsWith: []string{"tag", "virtual_machine"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"tag": {
				Type:     schema.TypeList,
				Optional: true,
				ConflictsWith: []string{
					"virtual_machine", "virtual_machine_group",
				},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"ip_address": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"https_redirect": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

func resourceLoadBalancerCreate(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	name := meta.UseOrGenerateName(d.Get("name").(string))

	t, ids := extractLoadBalancerResourceTypeAndIDs(d, m)
	if t == "" {
		t = katapult.VirtualMachinesResourceType
	}

	args := &katapult.LoadBalancerCreateArguments{
		Name:         name,
		ResourceType: t,
		ResourceIDs:  &ids,
		DataCenter:   meta.DataCenterRef(),
	}

	lb, _, err := meta.Client.LoadBalancers.Create(
		ctx, meta.OrganizationRef(), args,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(lb.ID)

	return resourceLoadBalancerRead(ctx, d, m)
}

func resourceLoadBalancerRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	c := m.(*Meta).Client
	var diags diag.Diagnostics

	id := d.Id()

	lb, resp, err := c.LoadBalancers.GetByID(ctx, id)
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == 404 {
			d.SetId("")

			return diags
		}

		return diag.FromErr(err)
	}

	_ = d.Set("name", lb.Name)
	_ = d.Set("resource_type", string(lb.ResourceType))
	populateLoadBalancerTargets(d, lb.ResourceType, lb.ResourceIDs)
	_ = d.Set("https_redirect", lb.HTTPSRedirect)
	if lb.IPAddress != nil {
		_ = d.Set("ip_address", lb.IPAddress.Address)
	}

	return diags
}

func resourceLoadBalancerUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	c := m.(*Meta).Client

	id := d.Id()

	lb := &katapult.LoadBalancer{ID: id}
	args := &katapult.LoadBalancerUpdateArguments{}

	if d.HasChange("name") {
		args.Name = d.Get("name").(string)
	}

	if d.HasChanges("virtual_machine", "virtual_machine_group", "tag") {
		t, ids := extractLoadBalancerResourceTypeAndIDs(d, m)
		args.ResourceType = t
		args.ResourceIDs = &ids
	}

	_, _, err := c.LoadBalancers.Update(ctx, lb, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceLoadBalancerRead(ctx, d, m)
}

func resourceLoadBalancerDelete(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	c := m.(*Meta).Client

	lb := &katapult.LoadBalancer{ID: d.Id()}

	_, _, err := c.LoadBalancers.Delete(ctx, lb)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}

func populateLoadBalancerTargets(
	d *schema.ResourceData,
	t katapult.ResourceType,
	ids []string,
) {
	list := []map[string]string{}
	for _, id := range ids {
		list = append(list, map[string]string{"id": id})
	}

	switch t {
	case katapult.VirtualMachinesResourceType:
		_ = d.Set("virtual_machine", list)
	case katapult.VirtualMachineGroupsResourceType:
		_ = d.Set("virtual_machine_group", list)
	case katapult.TagsResourceType:
		_ = d.Set("tag", list)
	}
}

func extractLoadBalancerResourceTypeAndIDs(
	d *schema.ResourceData,
	m interface{},
) (katapult.ResourceType, []string) {
	var t katapult.ResourceType
	var list []interface{}
	ids := []string{}

	if v, ok := d.GetOk("virtual_machine"); ok {
		t = katapult.VirtualMachinesResourceType
		list = v.([]interface{})
	} else if v, ok := d.GetOk("virtual_machine_group"); ok {
		t = katapult.VirtualMachineGroupsResourceType
		list = v.([]interface{})
	} else if v, ok := d.GetOk("tag"); ok {
		t = katapult.TagsResourceType
		list = v.([]interface{})
	}

	for _, item := range list {
		i := item.(map[string]interface{})
		ids = append(ids, i["id"].(string))
	}

	return t, ids
}
