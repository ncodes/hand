package constants

const (
	// WorkspaceMaxContentLength is the maximum instruction content loaded from workspace files.
	WorkspaceMaxContentLength = 15000
	// PersonalityFileName is the workspace personality file name.
	PersonalityFileName = "SOUL.md"
	// PersonalityMaxContentLength is the maximum content loaded from the personality file.
	PersonalityMaxContentLength = 15000
)

// WorkspaceDefaultInstructionFiles are the default workspace instruction file names.
var WorkspaceDefaultInstructionFiles = []string{"agents.md", "hand.md"}
