package v6provider

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityGroupMigrationCassetteMutationGuard(t *testing.T) {
	t.Parallel()
	mutations := securityGroupCassetteMutations(
		t, "testdata/SecurityGroup_migrate_v5_blocks_and_round_trip.cassette.yaml",
	)
	require.Equal(t, []string{
		"DELETE /security_groups/rules/security_group_rule",
		"DELETE /security_groups/security_group",
		"DELETE /security_groups/security_group",
		"DELETE /security_groups/security_group",
		"DELETE /security_groups/security_group",
		"DELETE /virtual_machine_groups/_",
		"PATCH /security_groups/rules/security_group_rule",
		"PATCH /security_groups/rules/security_group_rule",
		"POST /organizations/_/security_groups",
		"POST /organizations/_/security_groups",
		"POST /organizations/_/security_groups",
		"POST /organizations/_/security_groups",
		"POST /organizations/_/virtual_machine_groups",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
	}, mutations)
}

func TestSecurityGroupExternalRulesDisableToBlocksCassetteMutationGuard(t *testing.T) {
	t.Parallel()
	mutations := securityGroupCassetteMutations(
		t, "testdata/SecurityGroup_external_rules_disable_to_blocks.cassette.yaml",
	)
	require.Equal(t, []string{
		"DELETE /security_groups/security_group",
		"PATCH /security_groups/rules/security_group_rule",
		"PATCH /security_groups/rules/security_group_rule",
		"POST /organizations/organization/security_groups",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
	}, mutations)
}

func TestSecurityGroupExternalRulesDisableToPluralAttributesCassetteMutationGuard(t *testing.T) {
	t.Parallel()
	mutations := securityGroupCassetteMutations(
		t, "testdata/SecurityGroup_external_rules_disable_to_plural_attributes.cassette.yaml",
	)
	require.Equal(t, []string{
		"DELETE /security_groups/security_group",
		"PATCH /security_groups/rules/security_group_rule",
		"PATCH /security_groups/rules/security_group_rule",
		"POST /organizations/organization/security_groups",
		"POST /security_groups/{id}/rules",
		"POST /security_groups/{id}/rules",
	}, mutations)
}

func securityGroupCassetteMutations(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	securityGroupRulesPath := regexp.MustCompile(`/security_groups/[^/?]+/rules$`)
	mutations := []string{}
	requestURL := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "url: ") {
			requestURL = strings.TrimPrefix(line, "url: https://api.katapult.io/core/v1")
			if securityGroupRulesPath.MatchString(requestURL) {
				requestURL = securityGroupRulesPath.ReplaceAllString(requestURL, "/security_groups/{id}/rules")
			}
			if strings.HasPrefix(requestURL, "/virtual_machine_groups/_?") {
				requestURL = "/virtual_machine_groups/_"
			}
			continue
		}
		if line != "method: POST" && line != "method: PATCH" && line != "method: DELETE" {
			continue
		}
		mutations = append(mutations, strings.TrimPrefix(line, "method: ")+" "+requestURL)
	}
	require.NoError(t, scanner.Err())
	sort.Strings(mutations)
	return mutations
}
