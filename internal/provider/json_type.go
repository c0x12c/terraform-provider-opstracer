package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = jsonType{}
	_ basetypes.StringValuableWithSemanticEquals = jsonValue{}
	_ xattr.ValidateableAttribute                = jsonValue{}
)

// jsonType is a JSON object string. Values that differ only in key order, whitespace, number
// formatting, or null/empty fields the server fills in are treated as equal.
type jsonType struct {
	basetypes.StringType
}

func (t jsonType) String() string { return "jsonType" }

func (t jsonType) ValueType(_ context.Context) attr.Value { return jsonValue{} }

func (t jsonType) Equal(o attr.Type) bool {
	other, ok := o.(jsonType)
	return ok && t.StringType.Equal(other.StringType)
}

func (t jsonType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return jsonValue{StringValue: in}, nil
}

func (t jsonType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return jsonValue{StringValue: s}, nil
}

type jsonValue struct {
	basetypes.StringValue
}

func jsonValueOf(s string) jsonValue {
	return jsonValue{StringValue: basetypes.NewStringValue(s)}
}

func (v jsonValue) Type(_ context.Context) attr.Type { return jsonType{} }

func (v jsonValue) Equal(o attr.Value) bool {
	other, ok := o.(jsonValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v jsonValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	newValue, ok := newValuable.(jsonValue)
	if !ok {
		diags.AddError("Semantic equality check error", fmt.Sprintf("unexpected value type %T", newValuable))
		return false, diags
	}
	a, errA := canonicalJSON([]byte(v.ValueString()))
	b, errB := canonicalJSON([]byte(newValue.ValueString()))
	return errA == nil && errB == nil && a == b, diags
}

func (v jsonValue) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(v.ValueString()), &obj); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", err.Error())
	}
}

// canonicalJSON re-encodes with sorted keys and drops nulls and empty objects/arrays, so it
// matches what jsonencode() produces for a config that leaves those fields out.
func canonicalJSON(raw []byte) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	out, err := json.Marshal(prune(v))
	return string(out), err
}

func prune(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			child = prune(child)
			if isEmpty(child) {
				delete(t, k)
			} else {
				t[k] = child
			}
		}
	case []any:
		for i := range t {
			t[i] = prune(t[i])
		}
	}
	return v
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}
