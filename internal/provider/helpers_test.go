package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_stringsDiff(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "identical",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			want: []string{},
		},
		{
			name: "same elements, different order",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"wish", "pelf", "roster", "upscale", "pompano"},
			want: []string{},
		},
		{
			name: "more in a",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale"},
			want: []string{"pelf"},
		},
		{
			name: "more in b",
			a:    []string{"roster", "wish", "pompano", "upscale"},
			b:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			want: []string{},
		},
		{
			name: "most overlap",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale", "bale"},
			want: []string{"pelf"},
		},
		{
			name: "no overlap",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"ingest", "flambeau", "technic", "plinth", "rabid"},
			want: []string{"roster", "wish", "pompano", "upscale", "pelf"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsDiff(tt.a, tt.b)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Benchmark_stringsDiff(b *testing.B) {
	strs := []string{
		"roster", "wish", "pompano", "upscale", "pelf", "globule", "bale",
		"quizzed", "TIGRESS", "napoleon", "CAMEO", "jaguar", "chaperon",
		"ingest", "flambeau", "technic", "plinth", "rabid", "credo", "beau",
		"shrill", "lodgment", "saffron", "rattling", "tidings", "awhirl",
		"cloudlet", "oldest", "yacht", "trickle",
	}
	rem := []string{
		"parabola", "awhirl", "yacht", "beau", "ALAN", "credo", "cloudlet",
		"plinth", "wagon", "kepi", "trickle", "secede", "fum", "rabid",
		"homburg", "lodgment", "tidings", "catkin", "shrill", "when", "pippin",
		"saffron", "vasty", "commit", "rattling", "uprose", "oldest", "technic",
		"liter", "proxy",
	}

	for n := 0; n < b.N; n++ {
		stringsDiff(strs, rem)
	}
}

func Test_stringsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "identical",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			want: true,
		},
		{
			name: "same elements, different order",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"wish", "pelf", "roster", "upscale", "pompano"},
			want: true,
		},
		{
			name: "more in a",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale"},
			want: false,
		},
		{
			name: "more in b",
			a:    []string{"roster", "wish", "pompano", "upscale"},
			b:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			want: false,
		},
		{
			name: "most overlap",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"roster", "wish", "pompano", "upscale", "bale"},
			want: false,
		},
		{
			name: "no overlap",
			a:    []string{"roster", "wish", "pompano", "upscale", "pelf"},
			b:    []string{"ingest", "flambeau", "technic", "plinth", "rabid"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsEqual(tt.a, tt.b)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_mapKeys(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want []string
	}{
		{
			name: "empty map",
			m:    map[string]any{},
			want: []string{},
		},
		{
			name: "map with string values",
			m: map[string]any{
				"name":     "John",
				"age":      "30",
				"location": "NY",
			},
			want: []string{"name", "age", "location"},
		},
		{
			name: "map with mixed value types",
			m: map[string]any{
				"name":    "John",
				"age":     42,
				"active":  true,
				"hobbies": []string{"reading", "coding"},
				"address": map[string]string{"city": "NY"},
				"nothing": nil,
			},
			want: []string{
				"name",
				"age",
				"active",
				"hobbies",
				"address",
				"nothing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapKeys(tt.m)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
