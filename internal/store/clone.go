package store

import "aether-dots/internal/task"

// cloneDatabase copies the mutable slices and pointers in the JSON store while
// sharing immutable strings. This keeps snapshots and transactional changes
// proportional to the collection structure instead of JSON encoding every
// message on every read.
func cloneDatabase(source task.Database) task.Database {
	out := source
	out.Spaces = cloneSlice(source.Spaces)
	out.Dots = cloneSlice(source.Dots)
	for i := range out.Dots {
		out.Dots[i].Events = cloneSlice(source.Dots[i].Events)
		if source.Dots[i].Metrics != nil {
			metrics := *source.Dots[i].Metrics
			out.Dots[i].Metrics = &metrics
		}
	}
	out.Conversations = cloneSlice(source.Conversations)
	for i := range out.Conversations {
		conversation := &out.Conversations[i]
		original := source.Conversations[i]
		conversation.Messages = cloneSlice(original.Messages)
		conversation.DelegatedDotIDs = cloneSlice(original.DelegatedDotIDs)
		conversation.GitHubRepositoryIDs = cloneSlice(original.GitHubRepositoryIDs)
		conversation.GitHubWriteGrants = cloneSlice(original.GitHubWriteGrants)
		for j := range conversation.GitHubWriteGrants {
			conversation.GitHubWriteGrants[j].Kinds = cloneSlice(original.GitHubWriteGrants[j].Kinds)
		}
		conversation.ComputerUseScopes = cloneSlice(original.ComputerUseScopes)
		conversation.MCPToolGrants = cloneSlice(original.MCPToolGrants)
	}
	out.Skills = cloneSlice(source.Skills)
	out.Routines = cloneSlice(source.Routines)
	for i := range out.Routines {
		out.Routines[i].Runs = cloneSlice(source.Routines[i].Runs)
		out.Routines[i].RunStarts = cloneSlice(source.Routines[i].RunStarts)
	}
	out.RoutineRunStarts = cloneSlice(source.RoutineRunStarts)
	out.GitHubRepositories = cloneSlice(source.GitHubRepositories)
	out.GitHubProposals = cloneSlice(source.GitHubProposals)
	for i := range out.GitHubProposals {
		if source.GitHubProposals[i].Result != nil {
			result := *source.GitHubProposals[i].Result
			out.GitHubProposals[i].Result = &result
		}
	}
	out.MCPServers = cloneSlice(source.MCPServers)
	for i := range out.MCPServers {
		out.MCPServers[i].Tools = cloneSlice(source.MCPServers[i].Tools)
		for j := range out.MCPServers[i].Tools {
			out.MCPServers[i].Tools[j].InputSchema = cloneJSONMap(source.MCPServers[i].Tools[j].InputSchema)
		}
	}
	return out
}

func cloneSlice[T any](source []T) []T {
	if source == nil {
		return nil
	}
	return append([]T(nil), source...)
}

func cloneJSONMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		return cloneJSONMap(current)
	case []any:
		out := make([]any, len(current))
		for i, child := range current {
			out[i] = cloneJSONValue(child)
		}
		return out
	case []string:
		return cloneSlice(current)
	case []int64:
		return cloneSlice(current)
	default:
		return value
	}
}
