package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	TagResource struct {
		M *Meta
	}

	TagResourceModel struct {
		ID    types.String `json:"id"`
		Name  types.String `json:"name"`
		Color types.String `json:"color"`
	}
)

func (r *TagResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagResource) Configure(
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

func (r TagResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "The unique identifier for the tag.",
				MarkdownDescription: "The unique identifier for the tag.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The name of the tag.",
				MarkdownDescription: "The name of the tag.",
			},
			"color": schema.StringAttribute{
				Required: true,
				Description: "The color of the tag. Refer to " +
					"https://apidocs.k.io/katapult/enums/6808ef8ef6/ " +
					"for available colors",
				MarkdownDescription: "The color of the tag. Refer to " +
					"[the API documentation]" +
					"(https://apidocs.k.io/katapult/enums/6808ef8ef6/) " +
					"for available colors",
			},
		},
	}
}

func (r *TagResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.PostOrganizationTagsWithResponse(ctx,
		core.PostOrganizationTagsJSONRequestBody{
			Organization: core.OrganizationLookup{
				SubDomain: &r.M.confOrganization,
			},
			Properties: core.TagArguments{
				Name:  plan.Name.ValueStringPointer(),
				Color: (*core.TagColorsEnum)(plan.Color.ValueStringPointer()),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("Create Error", err.Error())
		return
	}

	id := res.JSON200.Tag.Id
	plan.ID = types.StringPointerValue(id)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	if err := r.TagRead(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *TagResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	state := TagResourceModel{}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.TagRead(ctx, state.ID.ValueStringPointer(), &state)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			r.M.Logger.Info(
				"Tag not found, removing from state",
				"id", state.ID.ValueString(),
			)

			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *TagResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := core.PatchTagJSONRequestBody{
		Tag: core.TagLookup{
			Id: state.ID.ValueStringPointer(),
		},

		Properties: core.TagArguments{},
	}

	if !plan.Name.Equal(state.Name) {
		args.Properties.Name = plan.Name.ValueStringPointer()
	}

	if !plan.Color.Equal(state.Color) {
		//nolint:lll // Generated enum conversion sucks
		args.Properties.Color = (*core.TagColorsEnum)(plan.Color.ValueStringPointer())
	}

	_, err := r.M.Core.PatchTagWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("Update Error", err.Error())
		return
	}

	if err := r.TagRead(ctx, state.ID.ValueStringPointer(), &plan); err != nil {
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *TagResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	state := TagResourceModel{}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteTagWithResponse(ctx,
		core.DeleteTagJSONRequestBody{
			Tag: core.TagLookup{
				Id: state.ID.ValueStringPointer(),
			},
		},
	)
	if err != nil {
		resp.Diagnostics.AddError("Delete Error", err.Error())
		return
	}
}

func (r *TagResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TagResource) TagRead(
	ctx context.Context,
	id *string,
	model *TagResourceModel,
) error {
	res, err := r.M.Core.GetTagWithResponse(ctx, &core.GetTagParams{
		TagId: id,
	})
	if err != nil {
		return err
	}

	model.ID = types.StringPointerValue(res.JSON200.Tag.Id)
	model.Name = types.StringPointerValue(res.JSON200.Tag.Name)
	model.Color = types.StringValue(string(*res.JSON200.Tag.Color))

	return nil
}
