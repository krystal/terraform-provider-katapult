package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	VirtualMachineGroupResource struct {
		M *Meta
	}

	VirtualMachineGroupResourceModel struct {
		ID        types.String `tfsdk:"id"`
		Name      types.String `tfsdk:"name"`
		Segregate types.Bool   `tfsdk:"segregate"`
	}
)

func (r *VirtualMachineGroupResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_group"
}

func (r *VirtualMachineGroupResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	r.M = meta
}

func (r *VirtualMachineGroupResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Virtual Machine Group in Katapult. " +
			"Virtual Machine Groups allow you to organize Virtual Machines " +
			"together and optionally segregate them across different host " +
			"machines for improved availability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The unique identifier of the " +
					"Virtual Machine Group.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the Virtual Machine Group.",
			},
			"segregate": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				MarkdownDescription: "When `true`, Katapult will attempt to " +
					"place Virtual Machines in this group on separate host " +
					"machines, providing hardware-level isolation for " +
					"improved availability. Defaults to `true`.",
			},
		},
	}
}

func (r *VirtualMachineGroupResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan VirtualMachineGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	segregate := plan.Segregate.ValueBool()
	res, err := r.M.Core.PostOrganizationVirtualMachineGroupsWithResponse(ctx,
		core.PostOrganizationVirtualMachineGroupsJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: core.VirtualMachineGroupArguments{
				Name:      plan.Name.ValueStringPointer(),
				Segregate: &segregate,
			},
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}

	vmg := res.JSON200.VirtualMachineGroup
	plan.ID = types.StringPointerValue(vmg.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	if err := r.vmgRead(ctx, vmg.Id, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VirtualMachineGroupResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state VirtualMachineGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.vmgRead(ctx, state.ID.ValueStringPointer(), &state)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *VirtualMachineGroupResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan VirtualMachineGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state VirtualMachineGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.VirtualMachineGroupArguments{}
	if !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueStringPointer()
	}
	if !plan.Segregate.Equal(state.Segregate) {
		segregate := plan.Segregate.ValueBool()
		args.Segregate = &segregate
	}

	res, err := r.M.Core.PatchVirtualMachineGroupWithResponse(ctx,
		core.PatchVirtualMachineGroupJSONRequestBody{
			VirtualMachineGroup: core.VirtualMachineGroupLookup{
				Id: state.ID.ValueStringPointer(),
			},
			Properties: args,
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.vmgRead(ctx, state.ID.ValueStringPointer(), &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VirtualMachineGroupResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state VirtualMachineGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.DeleteVirtualMachineGroupWithResponse(ctx,
		core.DeleteVirtualMachineGroupJSONRequestBody{
			VirtualMachineGroup: core.VirtualMachineGroupLookup{
				Id: state.ID.ValueStringPointer(),
			},
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
}

func (r *VirtualMachineGroupResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *VirtualMachineGroupResource) vmgRead(
	ctx context.Context,
	id *string,
	model *VirtualMachineGroupResourceModel,
) error {
	res, err := r.M.Core.GetVirtualMachineGroupWithResponse(ctx,
		&core.GetVirtualMachineGroupParams{
			VirtualMachineGroupId: id,
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return err
	}

	vmg := res.JSON200.VirtualMachineGroup
	model.ID = types.StringPointerValue(vmg.Id)
	model.Name = types.StringPointerValue(vmg.Name)
	model.Segregate = types.BoolPointerValue(vmg.Segregate)

	return nil
}
