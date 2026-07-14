package permissions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInventory_ReturnsValidDefensiveCopy(t *testing.T) {
	inventory := GetInventory()
	require.NotEmpty(t, inventory)

	seen := make(map[string]struct{}, len(inventory))
	for _, entry := range inventory {
		require.NotEmpty(t, entry.ID)
		require.Contains(t, []string{"tool", "rpc", "service"}, entry.Boundary)
		_, exists := seen[entry.ID]
		require.False(t, exists, entry.ID)
		seen[entry.ID] = struct{}{}
		_, err := entry.Operation.Normalize()
		require.NoError(t, err, entry.ID)
		if entry.Boundary == "tool" {
			require.Equal(t, entry.ID, "tool."+entry.Operation.Tool)
		}
	}

	inventory[0].ID = "changed"
	inventory[0].Operation.Effects[0] = EffectDestructive
	fresh := GetInventory()
	require.NotEqual(t, "changed", fresh[0].ID)
	require.NotEqual(t, EffectDestructive, fresh[0].Operation.Effects[0])
}

func TestInventory_CoversEveryBuiltInTool(t *testing.T) {
	want := []string{
		"tool.automation", "tool.list_files", "tool.memory_add", "tool.memory_delete", "tool.memory_extract",
		"tool.memory_search", "tool.memory_update", "tool.patch", "tool.plan_tool", "tool.process", "tool.read_file",
		"tool.run_command", "tool.search_files", "tool.session_messages", "tool.session_search", "tool.time",
		"tool.web_extract", "tool.web_search", "tool.write_file",
	}

	actual := make([]string, 0, len(want))
	for _, entry := range GetInventory() {
		if entry.Boundary == "tool" {
			actual = append(actual, entry.ID)
		}
	}
	require.ElementsMatch(t, want, actual)
}
