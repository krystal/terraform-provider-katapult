package main

import (
	"context"
	"slices"
	"testing"

	"github.com/krystal/terraform-provider-katapult/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderImplementationsCanBeMuxed(t *testing.T) {
	t.Setenv("TF_ACC", "")

	server, err := newProviderServer(context.Background())

	require.NoError(t, err,
		"legacy and Framework providers must not register the same type name")
	assert.NotNil(t, server)
}

func TestLegacyProviderRegistrations(t *testing.T) {
	t.Setenv("TF_ACC", "")

	legacyProvider := provider.New(&provider.Config{})()

	assert.Equal(t, []string{}, sortedKeys(legacyProvider.ResourcesMap),
		"new resources belong in internal/v6provider; "+
			"only remove entries during migration")

	assert.Equal(t, []string{
		"katapult_data_center",
		"katapult_disk_template",
		"katapult_disk_templates",
		"katapult_network_speed_profile",
		"katapult_network_speed_profiles",
		"katapult_virtual_machine_package",
		"katapult_virtual_machine_packages",
	}, sortedKeys(legacyProvider.DataSourcesMap),
		"new data sources belong in internal/v6provider; "+
			"only remove entries during migration")
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}
