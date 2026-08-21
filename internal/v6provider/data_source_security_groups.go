package v6provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	SecurityGroupDataSource      struct{ M *Meta }
	SecurityGroupRuleDataSource  struct{ M *Meta }
	SecurityGroupRulesDataSource struct{ M *Meta }
	SecurityGroupsDataSource     struct{ M *Meta }
)

type SecurityGroupDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Associations     types.Set    `tfsdk:"associations"`
	AllowAllInbound  types.Bool   `tfsdk:"allow_all_inbound"`
	AllowAllOutbound types.Bool   `tfsdk:"allow_all_outbound"`
	IncludeRules     types.Bool   `tfsdk:"include_rules"`
	InboundRules     types.List   `tfsdk:"inbound_rules"`
	OutboundRules    types.List   `tfsdk:"outbound_rules"`
}

type SecurityGroupRuleDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	Direction       types.String `tfsdk:"direction"`
	Protocol        types.String `tfsdk:"protocol"`
	Ports           types.String `tfsdk:"ports"`
	Targets         types.Set    `tfsdk:"targets"`
	Notes           types.String `tfsdk:"notes"`
}

type SecurityGroupRulesDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	InboundRules    types.List   `tfsdk:"inbound_rules"`
	OutboundRules   types.List   `tfsdk:"outbound_rules"`
}

type SecurityGroupsDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	IncludeRules   types.Bool   `tfsdk:"include_rules"`
	SecurityGroups types.List   `tfsdk:"security_groups"`
}

type securityGroupListModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Associations     types.Set    `tfsdk:"associations"`
	AllowAllInbound  types.Bool   `tfsdk:"allow_all_inbound"`
	AllowAllOutbound types.Bool   `tfsdk:"allow_all_outbound"`
	InboundRules     types.List   `tfsdk:"inbound_rules"`
	OutboundRules    types.List   `tfsdk:"outbound_rules"`
}

func configureSecurityGroupDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *Meta {
	if req.ProviderData == nil {
		return nil
	}
	m, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return nil
	}
	return m
}

func computedSecurityGroupRuleAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":        schema.StringAttribute{Computed: true},
		"direction": schema.StringAttribute{Computed: true},
		"protocol":  schema.StringAttribute{Computed: true},
		"ports":     schema.StringAttribute{Computed: true},
		"targets":   schema.SetAttribute{Computed: true, ElementType: types.StringType},
		"notes":     schema.StringAttribute{Computed: true},
	}
}

//nolint:lll // Nested schema is assembled in one place for parity.
func computedSecurityGroupAttributes(includeRules bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id":                               schema.StringAttribute{Computed: true},
		"name":                             schema.StringAttribute{Computed: true},
		securityGroupAssociationsAttribute: schema.SetAttribute{Computed: true, ElementType: types.StringType},
		"allow_all_inbound":                schema.BoolAttribute{Computed: true},
		"allow_all_outbound":               schema.BoolAttribute{Computed: true},
	}
	if includeRules {
		attrs["inbound_rules"] = schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: computedSecurityGroupRuleAttributes()}}
		attrs["outbound_rules"] = schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: computedSecurityGroupRuleAttributes()}}
	}
	return attrs
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group"
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.M = configureSecurityGroupDataSource(req, resp)
}

//nolint:lll // Compact schema supports direct parity review.
func (d *SecurityGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := computedSecurityGroupAttributes(true)
	attrs["id"] = schema.StringAttribute{Required: true}
	attrs["include_rules"] = schema.BoolAttribute{Optional: true}
	resp.Schema = schema.Schema{MarkdownDescription: "Retrieves a security group by ID.", Attributes: attrs}
}

func (d *SecurityGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecurityGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.IncludeRules.IsNull() {
		data.IncludeRules = types.BoolValue(true)
	}
	id := data.ID.ValueString()
	res, err := d.M.Core.GetSecurityGroupWithResponse(ctx, &core.GetSecurityGroupParams{SecurityGroupId: &id})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Read Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		resp.Diagnostics.AddError("Security Group Read Error", "security group not found")
		return
	}
	group := res.JSON200.SecurityGroup
	data.Name = types.StringPointerValue(group.Name)
	data.AllowAllInbound = types.BoolPointerValue(group.AllowAllInbound)
	data.AllowAllOutbound = types.BoolPointerValue(group.AllowAllOutbound)
	associations := []string{}
	if group.Associations != nil {
		associations = *group.Associations
	}
	data.Associations, _ = types.SetValueFrom(ctx, types.StringType, associations)
	if data.IncludeRules.ValueBool() {
		rules, readErr := getAllSecurityGroupRules(ctx, d.M, id)
		if readErr != nil {
			resp.Diagnostics.AddError("Security Group Rules Read Error", readErr.Error())
			return
		}
		data.InboundRules, data.OutboundRules, err = directionRuleValues(ctx, rules)
		if err != nil {
			resp.Diagnostics.AddError("Security Group Rules Read Error", err.Error())
			return
		}
	} else {
		data.InboundRules, data.OutboundRules, err = directionRuleValues(ctx, nil)
		if err != nil {
			resp.Diagnostics.AddError("Security Group Rules Read Error", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group_rule"
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.M = configureSecurityGroupDataSource(req, resp)
}

//nolint:lll // Compact schema supports direct parity review.
func (d *SecurityGroupRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := computedSecurityGroupRuleAttributes()
	attrs["id"] = schema.StringAttribute{Required: true}
	attrs["security_group_id"] = schema.StringAttribute{Computed: true}
	resp.Schema = schema.Schema{MarkdownDescription: "Retrieves a security group rule by ID.", Attributes: attrs}
}

//nolint:lll // Read maps the complete API projection.
func (d *SecurityGroupRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecurityGroupRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := data.ID.ValueString()
	res, err := d.M.Core.GetSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, &core.GetSecurityGroupsRulesSecurityGroupRuleParams{SecurityGroupRuleId: &id})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Rule Read Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		resp.Diagnostics.AddError("Security Group Rule Read Error", "security group rule not found")
		return
	}
	rule := res.JSON200.SecurityGroupRule
	if rule.SecurityGroup != nil {
		data.SecurityGroupID = types.StringPointerValue(rule.SecurityGroup.Id)
	}
	if rule.Direction != nil {
		data.Direction = types.StringValue(strings.ToLower(string(*rule.Direction)))
	}
	if rule.Protocol != nil {
		data.Protocol = types.StringValue(strings.ToUpper(string(*rule.Protocol)))
	}
	data.Ports, data.Notes = types.StringValue(nullableString(rule.Ports)), types.StringValue(nullableString(rule.Notes))
	targets := []string{}
	if rule.Targets != nil {
		targets = *rule.Targets
	}
	data.Targets, _ = types.SetValueFrom(ctx, types.StringType, targets)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupRulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group_rules"
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupRulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.M = configureSecurityGroupDataSource(req, resp)
}

//nolint:lll // Compact schema supports direct parity review.
func (d *SecurityGroupRulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Retrieves all rules for a security group.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "security_group_id": schema.StringAttribute{Required: true},
		"inbound_rules":  schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: computedSecurityGroupRuleAttributes()}},
		"outbound_rules": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: computedSecurityGroupRuleAttributes()}},
	}}
}

//nolint:lll // Read maps the complete API projection.
func (d *SecurityGroupRulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecurityGroupRulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := data.SecurityGroupID.ValueString()
	rules, err := getAllSecurityGroupRules(ctx, d.M, id)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Rules Read Error", err.Error())
		return
	}
	data.InboundRules, data.OutboundRules, err = directionRuleValues(ctx, rules)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Rules Read Error", err.Error())
		return
	}
	data.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func directionRuleValues(ctx context.Context, rules []canonicalSecurityGroupRule) (types.List, types.List, error) {
	inbound, outbound := []canonicalSecurityGroupRule{}, []canonicalSecurityGroupRule{}
	for _, rule := range rules {
		switch rule.Direction {
		case securityGroupDirectionInbound:
			inbound = append(inbound, rule)
		case securityGroupDirectionOutbound:
			outbound = append(outbound, rule)
		}
	}
	in, diags := securityGroupRulesListValue(ctx, inbound)
	if diags.HasError() {
		first := diags.Errors()[0]
		return in, types.ListNull(securityGroupRuleObjectType()), fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	out, diags := securityGroupRulesListValue(ctx, outbound)
	if diags.HasError() {
		first := diags.Errors()[0]
		return in, out, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	return in, out, nil
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_groups"
}

//nolint:lll // Framework interface signature.
func (d *SecurityGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.M = configureSecurityGroupDataSource(req, resp)
}

//nolint:lll // Compact schema supports direct parity review.
func (d *SecurityGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Retrieves all security groups in the organization.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "include_rules": schema.BoolAttribute{Optional: true},
		"security_groups": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: computedSecurityGroupAttributes(true)}},
	}}
}

//nolint:lll // Read maps the complete API projection.
func (d *SecurityGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SecurityGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.IncludeRules.IsNull() {
		data.IncludeRules = types.BoolValue(false)
	}
	groups, err := d.getAll(ctx, data.IncludeRules.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Security Groups Read Error", err.Error())
		return
	}
	data.SecurityGroups, resp.Diagnostics = types.ListValueFrom(ctx, types.ObjectType{AttrTypes: securityGroupListAttrTypes()}, groups)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(d.M.confOrganization)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

//nolint:lll // Object type mirrors the nested collection schema.
func securityGroupListAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id": types.StringType, "name": types.StringType, securityGroupAssociationsAttribute: types.SetType{ElemType: types.StringType},
		"allow_all_inbound": types.BoolType, "allow_all_outbound": types.BoolType,
		"inbound_rules": types.ListType{ElemType: securityGroupRuleObjectType()}, "outbound_rules": types.ListType{ElemType: securityGroupRuleObjectType()},
	}
}

//nolint:lll // Paginated expansion deliberately preserves API order.
func (d *SecurityGroupsDataSource) getAll(ctx context.Context, includeRules bool) ([]securityGroupListModel, error) {
	models := []securityGroupListModel{}
	totalPages := 1
	for page := 1; page <= totalPages; page++ {
		res, err := d.M.Core.GetOrganizationSecurityGroupsWithResponse(ctx, &core.GetOrganizationSecurityGroupsParams{OrganizationSubDomain: &d.M.confOrganization, Page: &page})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res == nil || res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing security groups")
		}
		for _, group := range res.JSON200.SecurityGroups {
			model := securityGroupListModel{ID: types.StringPointerValue(group.Id), Name: types.StringPointerValue(group.Name), AllowAllInbound: types.BoolPointerValue(group.AllowAllInbound), AllowAllOutbound: types.BoolPointerValue(group.AllowAllOutbound)}
			associations := []string{}
			if group.Associations != nil {
				associations = *group.Associations
			}
			model.Associations, _ = types.SetValueFrom(ctx, types.StringType, associations)
			if includeRules && group.Id != nil {
				rules, ruleErr := getAllSecurityGroupRules(ctx, d.M, *group.Id)
				if ruleErr != nil {
					return nil, ruleErr
				}
				model.InboundRules, model.OutboundRules, ruleErr = directionRuleValues(ctx, rules)
				if ruleErr != nil {
					return nil, ruleErr
				}
			} else {
				model.InboundRules, model.OutboundRules, err = directionRuleValues(ctx, nil)
				if err != nil {
					return nil, err
				}
			}
			models = append(models, model)
		}
		totalPages, err = securityGroupPaginationTotalPages(
			res.JSON200.Pagination, "security groups",
		)
		if err != nil {
			return nil, err
		}
	}
	return models, nil
}
