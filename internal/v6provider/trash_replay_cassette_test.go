package v6provider

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/stretchr/testify/require"
)

// Older cassettes have one final resource 404 for CheckDestroy. The provider now
// performs that check too. Duplicate only an already recorded terminal 404 in a
// temporary replay copy; never infer absence or change the tracked recording.
func trashDeletionReplayCassette(t *testing.T, name string) string {
	t.Helper()
	loaded, err := cassette.Load(name)
	if errors.Is(err, os.ErrNotExist) {
		// Configuration-only tests do not have recorded HTTP interactions.
		return name
	}
	require.NoError(t, err)
	count := len(loaded.Interactions)
	duplicateFinalDeletionChecks(loaded)
	if len(loaded.Interactions) == count {
		return name
	}
	loaded.Name = filepath.Join(t.TempDir(), "trash-deletion")
	loaded.File = loaded.Name + ".yaml"
	require.NoError(t, loaded.Save())
	return loaded.Name
}

func duplicateFinalDeletionChecks(loaded *cassette.Cassette) {
	last := make(map[string]*cassette.Interaction)
	for _, interaction := range loaded.Interactions {
		if interaction.Method == http.MethodGet {
			last[interaction.URL] = interaction
		}
	}
	for _, interaction := range loaded.Interactions {
		if interaction.Method != http.MethodGet || interaction.Code != http.StatusNotFound ||
			last[interaction.URL] != interaction {
			continue
		}
		parsed, err := url.Parse(interaction.URL)
		if err != nil {
			continue
		}
		switch parsed.Path {
		case "/core/v1/virtual_machines/virtual_machine",
			"/core/v1/disks/disk",
			"/core/v1/file_storage_volumes/file_storage_volume",
			"/core/v1/organizations/organization/object_storage/object_storage_cluster":
			duplicate := *interaction
			loaded.Interactions = append(loaded.Interactions, &duplicate)
		}
	}
}

func TestDuplicateFinalDeletionChecks(t *testing.T) {
	const endpoint = "https://api.example.test/core/v1/virtual_machines/virtual_machine?virtual_machine[id]=vm_test"
	tests := []struct {
		name     string
		endpoint string
		codes    []int
		extra    int
	}{
		{"terminal absence", endpoint, []int{200, 404}, 1},
		{"resource exists", endpoint, []int{404, 200}, 0},
		{"lookup fails", endpoint, []int{404, 403}, 0},
		{"unrelated endpoint", "https://api.example.test/core/v1/ip_addresses/ip_address", []int{404}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := &cassette.Cassette{}
			for _, code := range tt.codes {
				loaded.Interactions = append(loaded.Interactions, &cassette.Interaction{
					Request:  cassette.Request{Method: http.MethodGet, URL: tt.endpoint},
					Response: cassette.Response{Code: code, Body: "recorded body"},
				})
			}
			duplicateFinalDeletionChecks(loaded)
			require.Len(t, loaded.Interactions, len(tt.codes)+tt.extra)
			if tt.extra > 0 {
				require.Equal(t, loaded.Interactions[len(tt.codes)-1], loaded.Interactions[len(tt.codes)])
				require.NotSame(t, loaded.Interactions[len(tt.codes)-1], loaded.Interactions[len(tt.codes)])
			}
		})
	}
}

func TestTrashDeletionReplayCassettePreservesSource(t *testing.T) {
	source := cassette.New(filepath.Join(t.TempDir(), "source"))
	source.Interactions = []*cassette.Interaction{{
		Request: cassette.Request{
			Method: http.MethodGet, URL: "https://api.example.test/core/v1/disks/disk?disk[id]=disk_test",
		},
		Response: cassette.Response{Code: http.StatusNotFound},
	}}
	require.NoError(t, source.Save())
	before, err := os.ReadFile(source.File)
	require.NoError(t, err)
	replayName := trashDeletionReplayCassette(t, source.Name)
	require.NotEqual(t, source.Name, replayName)
	replay, err := cassette.Load(replayName)
	require.NoError(t, err)
	require.Len(t, replay.Interactions, 2)
	after, err := os.ReadFile(source.File)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
