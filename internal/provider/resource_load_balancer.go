package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

//nolint:unused
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

//nolint:unused
func resourceLoadBalancerCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	name := m.UseOrGenerateName(d.Get("name").(string))

	t, ids := extractLoadBalancerResourceTypeAndIDs(d)
	if t == "" {
		t = core.VirtualMachinesResourceType
	}

	args := &core.LoadBalancerCreateArguments{
		Name:         name,
		ResourceType: t,
		ResourceIDs:  &ids,
		DataCenter:   m.DataCenterRef,
	}

	lb, _, err := m.Core.LoadBalancers.Create(
		ctx, m.OrganizationRef, args,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(lb.ID)

	return resourceLoadBalancerRead(ctx, d, meta)
}

//nolint:unused
func resourceLoadBalancerRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Id()

	lb, _, err := m.Core.LoadBalancers.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
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

//nolint:unused
func resourceLoadBalancerUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	id := d.Id()

	lbRef := core.LoadBalancerRef{ID: id}
	args := &core.LoadBalancerUpdateArguments{}

	if d.HasChange("name") {
		args.Name = d.Get("name").(string)
	}

	if d.HasChanges("virtual_machine", "virtual_machine_group", "tag") {
		t, ids := extractLoadBalancerResourceTypeAndIDs(d)
		args.ResourceType = t
		args.ResourceIDs = &ids
	}

	_, _, err := m.Core.LoadBalancers.Update(ctx, lbRef, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceLoadBalancerRead(ctx, d, meta)
}

//nolint:unused
func resourceLoadBalancerDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	lbRef := core.LoadBalancerRef{ID: d.Id()}

	_, _, err := m.Core.LoadBalancers.Delete(ctx, lbRef)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}

//nolint:unused
func populateLoadBalancerTargets(
	d *schema.ResourceData,
	t core.ResourceType,
	ids []string,
) {
	list := flattenLoadBalancerResourceIDs(ids)

	switch t {
	case core.VirtualMachinesResourceType:
		_ = d.Set("virtual_machine", list)
	case core.VirtualMachineGroupsResourceType:
		_ = d.Set("virtual_machine_group", list)
	case core.TagsResourceType:
		_ = d.Set("tag", list)
	}
}

//nolint:unused
func flattenLoadBalancerResourceIDs(ids []string) []map[string]string {
	list := []map[string]string{}
	for _, id := range ids {
		list = append(list, map[string]string{"id": id})
	}

	return list
}

//nolint:unused
func extractLoadBalancerResourceTypeAndIDs(
	d *schema.ResourceData,
) (core.ResourceType, []string) {
	var t core.ResourceType
	var list []interface{}
	ids := []string{}

	if v, ok := d.GetOk("virtual_machine"); ok {
		t = core.VirtualMachinesResourceType
		list = v.([]interface{})
	} else if v, ok := d.GetOk("virtual_machine_group"); ok {
		t = core.VirtualMachineGroupsResourceType
		list = v.([]interface{})
	} else if v, ok := d.GetOk("tag"); ok {
		t = core.TagsResourceType
		list = v.([]interface{})
	}

	for _, item := range list {
		i := item.(map[string]interface{})
		ids = append(ids, i["id"].(string))
	}

	return t, ids
}
