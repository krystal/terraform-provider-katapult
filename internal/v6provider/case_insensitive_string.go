package v6provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type caseInsensitiveStringType struct{ basetypes.StringType }

func (t caseInsensitiveStringType) Equal(other attr.Type) bool {
	_, ok := other.(caseInsensitiveStringType)
	return ok
}

func (caseInsensitiveStringType) String() string { return "CaseInsensitiveStringType" }

//nolint:lll // Framework custom type interface signature.
func (t caseInsensitiveStringType) ValueFromString(_ context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return caseInsensitiveStringValue{StringValue: value}, nil
}

func (t caseInsensitiveStringType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	base, err := t.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}
	stringValue, ok := base.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", base)
	}
	return caseInsensitiveStringValue{StringValue: stringValue}, nil
}

func (caseInsensitiveStringType) ValueType(context.Context) attr.Value {
	return caseInsensitiveStringValue{StringValue: types.StringNull()}
}

type caseInsensitiveStringValue struct{ basetypes.StringValue }

func (v caseInsensitiveStringValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(caseInsensitiveStringValue)
	return ok && v.StringValue.Equal(otherValue.StringValue)
}

func (v caseInsensitiveStringValue) Type(context.Context) attr.Type {
	return caseInsensitiveStringType{}
}

//nolint:lll // Framework custom type interface signature.
func (v caseInsensitiveStringValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() || other.IsNull() || other.IsUnknown() {
		return false, nil
	}
	otherValue, diags := other.ToStringValue(ctx)
	return strings.EqualFold(v.ValueString(), otherValue.ValueString()), diags
}

func caseInsensitiveStringValueOf(value string) caseInsensitiveStringValue {
	return caseInsensitiveStringValue{StringValue: types.StringValue(value)}
}
