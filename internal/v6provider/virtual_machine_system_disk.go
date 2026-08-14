package v6provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/krystal/go-katapult/next/core"
)

type VirtualMachineSystemDiskModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	SizeInGB     types.Int64  `tfsdk:"size_in_gb"`
	ResizeMethod types.String `tfsdk:"resize_method"`
	WWN          types.String `tfsdk:"wwn"`
	State        types.String `tfsdk:"state"`
}

//nolint:goconst // Terraform attribute names are clearer inline in the shared type map.
var virtualMachineSystemDiskAttrTypes = map[string]attr.Type{
	"id":               types.StringType,
	"name":             types.StringType,
	"size_in_gb":       types.Int64Type,
	"resize_method":    types.StringType,
	"wwn":              types.StringType,
	stateAttributeName: types.StringType,
}

//nolint:lll // Generated framework value types make the conversion signature long.
func decodeVirtualMachineSystemDisk(ctx context.Context, value types.Object) (VirtualMachineSystemDiskModel, diag.Diagnostics) {
	var model VirtualMachineSystemDiskModel
	if value.IsNull() || value.IsUnknown() {
		return model, nil
	}
	diags := value.As(ctx, &model, basetypes.ObjectAsOptions{})
	return model, diags
}

//nolint:lll // Generated framework value types make the conversion signature long.
func virtualMachineSystemDiskValue(ctx context.Context, model VirtualMachineSystemDiskModel) (types.Object, diag.Diagnostics) {
	return types.ObjectValueFrom(ctx, virtualMachineSystemDiskAttrTypes, model)
}

func selectBootDiskAssignment(
	attachments []core.GetVirtualMachineDisks200ResponseDisks,
	priorID string,
) (*core.GetVirtualMachineDisks200ResponseDisks, bool) {
	var explicit []*core.GetVirtualMachineDisks200ResponseDisks
	var nilBoot []*core.GetVirtualMachineDisks200ResponseDisks
	for i := range attachments {
		a := &attachments[i]
		if a.Boot == nil {
			nilBoot = append(nilBoot, a)
			continue
		}
		if *a.Boot {
			explicit = append(explicit, a)
		}
	}
	if len(explicit) == 1 {
		return explicit[0], true
	}
	if len(explicit) > 1 {
		return nil, false
	}
	if priorID != "" {
		for i := range attachments {
			a := &attachments[i]
			if a.Disk != nil && a.Disk.Id != nil && *a.Disk.Id == priorID {
				return a, true
			}
		}
	}
	if len(nilBoot) == 1 {
		return nilBoot[0], true
	}
	return nil, false
}

func fetchVirtualMachineBootDisk(
	ctx context.Context,
	m *Meta,
	vmID, priorID string,
) (*core.GetDisk200ResponseDisk, error) {
	attachments, err := fetchAllVMDisks(ctx, m, vmID)
	if err != nil {
		return nil, err
	}
	selected, authoritative := selectBootDiskAssignment(attachments, priorID)
	if !authoritative || selected == nil || selected.Disk == nil || selected.Disk.Id == nil {
		return nil, fmt.Errorf("could not identify one authoritative boot disk for Virtual Machine %s", vmID)
	}
	diskID := *selected.Disk.Id
	res, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &diskID})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return nil, err
	}
	if res == nil || res.JSON200 == nil {
		return nil, fmt.Errorf("unexpected empty response fetching boot disk %s", diskID)
	}
	return &res.JSON200.Disk, nil
}

func populateVirtualMachineSystemDisk(
	ctx context.Context,
	model *VirtualMachineResourceModel,
	disk *core.GetDisk200ResponseDisk,
) diag.Diagnostics {
	prior, diags := decodeVirtualMachineSystemDisk(ctx, model.SystemDisk)
	if diags.HasError() {
		return diags
	}
	method := prior.ResizeMethod
	if method.IsNull() || method.IsUnknown() || method.ValueString() == "" {
		method = types.StringValue("offline")
	}
	next := VirtualMachineSystemDiskModel{
		ID:           types.StringPointerValue(disk.Id),
		Name:         types.StringPointerValue(disk.Name),
		SizeInGB:     types.Int64Null(),
		ResizeMethod: method,
		WWN:          types.StringPointerValue(disk.Wwn),
		State:        types.StringNull(),
	}
	if disk.SizeInGb != nil {
		next.SizeInGB = types.Int64Value(int64(*disk.SizeInGb))
	}
	if disk.State != nil {
		next.State = types.StringValue(string(*disk.State))
	}
	model.SystemDisk, diags = virtualMachineSystemDiskValue(ctx, next)
	return diags
}

func virtualMachineDiskTemplateRefsEquivalent(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if strings.HasPrefix(a, "dtpl_") || strings.HasPrefix(b, "dtpl_") {
		return false
	}
	return strings.TrimPrefix(a, "templates/") == strings.TrimPrefix(b, "templates/")
}

func fetchDiskTemplateReferences(ctx context.Context, m *Meta, diskID string) ([]string, error) {
	if diskID == "" {
		return nil, nil
	}
	res, err := m.Core.GetDiskWithResponse(ctx, &core.GetDiskParams{DiskId: &diskID})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return nil, err
	}
	if res == nil || res.JSON200 == nil {
		return nil, fmt.Errorf("unexpected empty response fetching boot disk %s", diskID)
	}
	installationValue := res.JSON200.Disk.Installation
	if !installationValue.IsSpecified() || installationValue.IsNull() {
		return nil, nil
	}
	installation, err := installationValue.Get()
	if err != nil || installation.DiskTemplateVersion == nil ||
		installation.DiskTemplateVersion.DiskTemplate == nil {
		return nil, err
	}
	template := installation.DiskTemplateVersion.DiskTemplate
	refs := make([]string, 0, 2)
	if template.Id != nil {
		refs = append(refs, *template.Id)
	}
	if template.Permalink != nil {
		refs = append(refs, *template.Permalink)
	}
	return refs, nil
}

func patchDiskName(ctx context.Context, m *Meta, diskID, name string) error {
	res, err := m.Core.PatchDiskWithResponse(ctx, core.PatchDiskJSONRequestBody{
		Disk:       core.DiskLookup{Id: &diskID},
		Properties: core.DiskArguments{Name: &name},
	})
	if err != nil {
		if res != nil {
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res == nil || res.JSON200 == nil {
		return fmt.Errorf("unexpected empty response renaming disk %s", diskID)
	}
	return nil
}

//nolint:lll // Keep the resize operation boundary explicit at its signature.
func resizeDiskAndWait(ctx context.Context, m *Meta, diskID string, size int64, method core.ResizeMethodEnum, timeoutDuration time.Duration) error {
	newSize := int(size)
	res, err := m.Core.PutDiskResizeWithResponse(ctx, core.PutDiskResizeJSONRequestBody{
		Disk: core.DiskLookup{Id: &diskID}, SizeInGb: newSize, ResizeMethod: &method,
	})
	if err != nil {
		if res != nil {
			return genericAPIError(err, res.Body)
		}
		return err
	}
	if res == nil || res.JSON200 == nil || res.JSON200.Task.Id == nil {
		return fmt.Errorf("unexpected empty response resizing disk %s", diskID)
	}
	if err := waitForTaskCompletion(ctx, m, timeoutDuration, *res.JSON200.Task.Id); err != nil {
		return err
	}
	return waitForDiskSize(ctx, m, diskID, size, timeoutDuration)
}
