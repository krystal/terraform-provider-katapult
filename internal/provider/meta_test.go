package provider

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeta_UseOrGenerateName(t *testing.T) {
	type fields struct {
		GeneratedNamePrefix string
	}
	type args struct {
		name string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *regexp.Regexp
	}{
		{
			name:   "no prefix and empty string",
			fields: fields{},
			args:   args{name: ""},
			want:   regexp.MustCompile("^[^-]+-[^-]+$"),
		},
		{
			name:   "no prefix and non-empty string",
			fields: fields{},
			args:   args{name: "dope-groovy-narwhal-flower"},
			want:   regexp.MustCompile("^dope-groovy-narwhal-flower$"),
		},
		{
			name:   "prefix and empty string",
			fields: fields{GeneratedNamePrefix: "tf-unit-test"},
			args:   args{name: ""},
			want:   regexp.MustCompile("^tf-unit-test-[^-]+-[^-]+$"),
		},
		{
			name:   "prefix and non-empty string",
			fields: fields{GeneratedNamePrefix: "tf-unit-test"},
			args:   args{name: "dope-groovy-narwhal-flower"},
			want:   regexp.MustCompile("^dope-groovy-narwhal-flower$"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Meta{
				GeneratedNamePrefix: tt.fields.GeneratedNamePrefix,
			}

			got := m.UseOrGenerateName(tt.args.name)

			assert.Regexp(t, tt.want, got)
		})
	}
}

func TestMeta_UseOrGenerateHostname(t *testing.T) {
	type fields struct {
		GeneratedNamePrefix string
	}
	type args struct {
		name string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *regexp.Regexp
	}{
		{
			name:   "no prefix and empty string",
			fields: fields{},
			args:   args{name: ""},
			want:   regexp.MustCompile("^[^-]+-[^-]+-[^-]+$"),
		},
		{
			name:   "no prefix and non-empty string",
			fields: fields{},
			args:   args{name: "dope-groovy-narwhal-flower"},
			want:   regexp.MustCompile("^dope-groovy-narwhal-flower$"),
		},
		{
			name:   "prefix and empty string",
			fields: fields{GeneratedNamePrefix: "tf-unit-test"},
			args:   args{name: ""},
			want:   regexp.MustCompile("^tf-unit-test-[^-]+-[^-]+-[^-]+$"),
		},
		{
			name:   "prefix and non-empty string",
			fields: fields{GeneratedNamePrefix: "tf-unit-test"},
			args:   args{name: "dope-groovy-narwhal-flower"},
			want:   regexp.MustCompile("^dope-groovy-narwhal-flower$"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Meta{
				GeneratedNamePrefix: tt.fields.GeneratedNamePrefix,
			}

			got := m.UseOrGenerateHostname(tt.args.name)

			assert.Regexp(t, tt.want, got)
		})
	}
}
