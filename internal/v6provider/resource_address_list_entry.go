package v6provider

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	AddressListEntryResource struct {
		M *Meta
	}

	AddressListEntryResourceModel struct {
		ID            types.String `tfsdk:"id"`
		AddressListID types.String `tfsdk:"address_list_id"`
		Name          types.String `tfsdk:"name"`
		Address       types.String `tfsdk:"address"`
	}
)

func (r *AddressListEntryResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_list_entry"
}

func (r *AddressListEntryResource) Configure(
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

func (r *AddressListEntryResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"address_list_id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"address": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *AddressListEntryResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	plan := AddressListEntryResourceModel{}

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.PostAddressListEntriesWithResponse(ctx,
		core.PostAddressListEntriesJSONRequestBody{
			AddressList: core.AddressListLookup{
				Id: plan.AddressListID.ValueStringPointer(),
			},
			Properties: core.AddressListEntryArguments{
				Name:    plan.Name.ValueStringPointer(),
				Address: plan.Address.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("create error", err.Error())

		return
	}
	if res.StatusCode() < 200 || res.StatusCode() >= 300 {
		resp.Diagnostics.AddError(
			"Address List Entry Create Error",
			string(res.Body),
		)
		return
	}

	if res.JSON201.AddressListEntry.Id == nil {
		resp.Diagnostics.AddError(
			"Address List Entry Create Error",
			"missing ID in response",
		)
		return
	}

	id := *res.JSON201.AddressListEntry.Id

	if err := r.AddressListEntryRead(ctx, id, &plan, &resp.State); err != nil {
		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AddressListEntryResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var model AddressListEntryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := model.ID.ValueString()
	err := r.AddressListEntryRead(ctx, id, &model, &resp.State)
	if err != nil {
		resp.Diagnostics.AddError("Address List Entry Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *AddressListEntryResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan AddressListEntryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AddressListEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.PatchAddressListEntryWithResponse(ctx,
		core.PatchAddressListEntryJSONRequestBody{
			AddressListEntry: core.AddressListEntryLookup{
				Id: state.ID.ValueStringPointer(),
			},
			Properties: core.AddressListEntryArguments{
				Name:    plan.Name.ValueStringPointer(),
				Address: plan.Address.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("update error", err.Error())

		return
	}

	if res.StatusCode() < 200 || res.StatusCode() >= 300 {
		resp.Diagnostics.AddError(
			"Address Entry List Update Error",
			string(res.Body),
		)
		return
	}

	id := state.ID.ValueString()
	err = r.AddressListEntryRead(ctx, id, &plan, &resp.State)
	if err != nil {
		resp.Diagnostics.AddError("Address List Entry Read Error", err.Error())
		return
	}

	spew.Dump(plan)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AddressListEntryResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var model AddressListEntryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteAddressListEntryWithResponse(ctx,
		core.DeleteAddressListEntryJSONRequestBody{
			AddressListEntry: core.AddressListEntryLookup{
				Id: model.ID.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("delete error", err.Error())
		return
	}
}

func (r *AddressListEntryResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AddressListEntryResource) AddressListEntryRead(
	ctx context.Context,
	id string,
	model *AddressListEntryResourceModel,
	state *tfsdk.State,
) error {
	res, err := r.M.Core.GetAddressListEntryWithResponse(ctx,
		&core.GetAddressListEntryParams{
			AddressListEntryId: &id,
		})
	if err != nil {
		if res.JSON404 != nil {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

	if res.JSON200 == nil {
		return fmt.Errorf("no address list entry found with ID %s", id)
	}

	entry := res.JSON200.AddressListEntry

	model.ID = types.StringPointerValue(entry.Id)
	model.Name = types.StringPointerValue(entry.Name)
	model.Address = types.StringPointerValue(entry.Address)

	return nil
}
