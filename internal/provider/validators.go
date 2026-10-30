package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// httpURLPattern matches the http and https URLs Domain Chief accepts as redirect target.
var httpURLPattern = regexp.MustCompile(`^https?://\S+$`)

// noPrefixValidator rejects strings that start with a reserved prefix.
type noPrefixValidator struct {
	prefix string
}

var _ validator.String = noPrefixValidator{}

func (v noPrefixValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must not start with %q", v.prefix)
}

func (v noPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v noPrefixValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if strings.HasPrefix(req.ConfigValue.ValueString(), v.prefix) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Reserved metadata key",
			fmt.Sprintf("Metadata keys starting with %q are managed by Domain Chief and cannot be set.", v.prefix),
		)
	}
}
