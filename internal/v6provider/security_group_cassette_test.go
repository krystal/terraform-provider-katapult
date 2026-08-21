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

	file, err := os.Open("testdata/SecurityGroup_migrate_v5_blocks_and_round_trip.cassette.yaml")
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
