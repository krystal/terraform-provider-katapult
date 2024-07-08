package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	AddressListResource struct {
		M *Meta
	}

	AddressListResourceModel struct {
		ID   types.String `tfsdk:"id"`
		Name types.String `tfsdk:"name"`
	}
)

func (r *AddressListResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_list"
}

func (r *AddressListResource) Configure(
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

func (r *AddressListResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *AddressListResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan AddressListResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.PostOrganizationAddressListsWithResponse(ctx,
		core.PostOrganizationAddressListsJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: core.AddressListArguments{
				Name: plan.Name.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("create error", err.Error())
		return
	}

	if res.JSON201.AddressList.Id == nil {
		resp.Diagnostics.AddError(
			"Address List Create Error",
			"missing ID in response",
		)
		return
	}

	id := *res.JSON201.AddressList.Id

	if err := r.AddressListRead(ctx, id, &plan, &resp.State); err != nil {
		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AddressListResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var model AddressListResourceModel
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.AddressListRead(ctx, model.ID.ValueString(), &model, &resp.State)
	if err != nil {
		resp.Diagnostics.AddError("Address List Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *AddressListResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan AddressListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AddressListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.PatchAddressListWithResponse(ctx,
		core.PatchAddressListJSONRequestBody{
			AddressList: core.AddressListLookup{
				Id: state.ID.ValueStringPointer(),
			},
			Properties: core.AddressListArguments{
				Name: plan.Name.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("update error", err.Error())
		return
	}

	err = r.AddressListRead(ctx, state.ID.ValueString(), &plan, &resp.State)
	if err != nil {
		resp.Diagnostics.AddError("Address List Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AddressListResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state AddressListResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteAddressListWithResponse(ctx,
		core.DeleteAddressListJSONRequestBody{
			AddressList: core.AddressListLookup{
				Id: state.ID.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("delete error", err.Error())
		return
	}
}

func (r *AddressListResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AddressListResource) AddressListRead(
	ctx context.Context,
	id string,
	model *AddressListResourceModel,
	state *tfsdk.State,
) error {
	res, err := r.M.Core.GetAddressListWithResponse(ctx,
		&core.GetAddressListParams{
			AddressListId: &id,
		})
	if err != nil {
		if res.JSON404 != nil {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

	addressList := res.JSON200.AddressList

	model.ID = types.StringPointerValue(addressList.Id)
	model.Name = types.StringPointerValue(addressList.Name)

	return nil
}
