package v6provider

import (
	"testing"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
)

func TestIsAdditionalDiskAttachment(t *testing.T) {
	t.Parallel()

	trueValue := true
	falseValue := false

	tests := []struct {
		name       string
		attachment core.GetVirtualMachineDisks200ResponseDisks
		want       bool
	}{
		{
			name: "explicit additional disk",
			attachment: core.GetVirtualMachineDisks200ResponseDisks{
				Boot: &falseValue,
			},
			want: true,
		},
		{
			name: "explicit boot disk",
			attachment: core.GetVirtualMachineDisks200ResponseDisks{
				Boot: &trueValue,
			},
			want: false,
		},
		{
			name:       "missing boot flag treated as boot",
			attachment: core.GetVirtualMachineDisks200ResponseDisks{},
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, isAdditionalDiskAttachment(test.attachment))
		})
	}
}
