package permissions

import "strings"

type InventoryEntry struct {
	ID        string
	Boundary  string
	Operation Operation
}

var operationInventory = []InventoryEntry{
	{
		ID:       "tool.list_files",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceFile,
			Action:   ActionList,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.read_file",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceFile,
			Action:   ActionRead,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.search_files",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceFile,
			Action:   ActionSearch,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.write_file",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceFile,
			Action:   ActionUpdate,
			Effects:  []Effect{EffectWrite},
		},
	},
	{
		ID:       "tool.patch",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceFile,
			Action:   ActionUpdate,
			Effects:  []Effect{EffectWrite},
		},
	},
	{
		ID:       "tool.run_command",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceProcess,
			Action:   ActionExecute,
			Effects:  []Effect{EffectExecution},
		},
	},
	{
		ID:       "tool.process",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceProcess,
			Action:   ActionManage,
			Effects:  []Effect{EffectRead, EffectWrite, EffectExecution},
		},
	},
	{
		ID:       "tool.web_search",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceNetwork,
			Action:   ActionSearch,
			Effects:  []Effect{EffectRead, EffectNetwork, EffectExternalSystem},
		},
	},
	{
		ID:       "tool.web_extract",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceNetwork,
			Action:   ActionRead,
			Effects:  []Effect{EffectRead, EffectNetwork, EffectExternalSystem},
		},
	},
	{
		ID:       "tool.memory_search",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceMemory,
			Action:   ActionSearch,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.memory_extract",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceMemory,
			Action:   ActionRead,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.memory_add",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceMemory,
			Action:   ActionCreate,
			Effects:  []Effect{EffectWrite},
		},
	},
	{
		ID:       "tool.memory_update",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceMemory,
			Action:   ActionUpdate,
			Effects:  []Effect{EffectWrite},
		},
	},
	{
		ID:       "tool.memory_delete",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceMemory,
			Action:   ActionDelete,
			Effects:  []Effect{EffectWrite, EffectDestructive},
		},
	},
	{
		ID:       "tool.session_messages",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceSession,
			Action:   ActionRead,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.session_search",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceSession,
			Action:   ActionSearch,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "tool.automation",
		Boundary: "tool",
		Operation: Operation{
			Resource:      ResourceAutomation,
			Action:        ActionManage,
			Effects:       []Effect{EffectRead, EffectWrite, EffectExternalSystem},
			OwnerRequired: true,
		},
	},
	{
		ID:       "tool.plan_tool",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourcePlan,
			Action:   ActionManage,
			Effects:  []Effect{EffectRead, EffectWrite},
		},
	},
	{
		ID:       "tool.time",
		Boundary: "tool",
		Operation: Operation{
			Resource: ResourceClock,
			Action:   ActionRead,
			Effects:  []Effect{EffectRead},
		},
	},
	{
		ID:       "rpc.session_mutation",
		Boundary: "rpc",
		Operation: Operation{
			Resource: ResourceSession,
			Action:   ActionManage,
			Effects:  []Effect{EffectWrite},
		},
	},
	{
		ID:       "rpc.model_selection",
		Boundary: "rpc",
		Operation: Operation{
			Resource:      ResourceModel,
			Action:        ActionUpdate,
			Effects:       []Effect{EffectWrite},
			OwnerRequired: true,
		},
	},
	{
		ID:       "rpc.provider_credential",
		Boundary: "rpc",
		Operation: Operation{
			Resource:      ResourceConfiguration,
			Action:        ActionUpdate,
			Effects:       []Effect{EffectWrite, EffectCredentialBearing},
			OwnerRequired: true,
		},
	},
	{
		ID:       "rpc.gateway_lifecycle",
		Boundary: "rpc",
		Operation: Operation{
			Resource:      ResourceGateway,
			Action:        ActionManage,
			Effects:       []Effect{EffectWrite, EffectNetwork},
			OwnerRequired: true,
		},
	},
	{
		ID:       "rpc.gateway_pairing",
		Boundary: "rpc",
		Operation: Operation{
			Resource:      ResourceGateway,
			Action:        ActionManage,
			Effects:       []Effect{EffectWrite, EffectPrivilegeChanging},
			OwnerRequired: true,
		},
	},
	{
		ID:       "rpc.automation_mutation",
		Boundary: "rpc",
		Operation: Operation{
			Resource:      ResourceAutomation,
			Action:        ActionManage,
			Effects:       []Effect{EffectWrite, EffectExternalSystem},
			OwnerRequired: true,
		},
	},
	{
		ID:       "service.automation_execution",
		Boundary: "service",
		Operation: Operation{
			Resource: ResourceAutomation,
			Action:   ActionExecute,
			Effects:  []Effect{EffectExecution, EffectExternalSystem},
		},
	},
	{
		ID:       "service.daemon_lifecycle",
		Boundary: "service",
		Operation: Operation{
			Resource:      ResourceDaemon,
			Action:        ActionManage,
			Effects:       []Effect{EffectWrite, EffectPrivilegeChanging},
			OwnerRequired: true,
		},
	},
}

func GetInventory() []InventoryEntry {
	result := make([]InventoryEntry, len(operationInventory))
	copy(result, operationInventory)
	for index := range result {
		result[index].Operation.Effects = append([]Effect(nil), result[index].Operation.Effects...)
		if result[index].Boundary == "tool" && result[index].Operation.Tool == "" {
			result[index].Operation.Tool = strings.TrimPrefix(result[index].ID, "tool.")
		}
	}
	return result
}
