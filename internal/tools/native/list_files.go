package native

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/tools"
)

func ListFilesDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		Path          string `json:"path"`
		Recursive     *bool  `json:"recursive"`
		IncludeHidden bool   `json:"include_hidden"`
		MaxEntries    int    `json:"max_entries"`
	}

	type entry struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size,omitempty"`
	}

	return tools.Definition{
		Name:        "list_files",
		Description: "List files and directories under an allowed workspace root.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Filesystem: true},
		InputSchema: objectSchema(map[string]any{
			"path":           stringSchema("Path relative to an allowed workspace root. Defaults to the workspace root when omitted."),
			"recursive":      booleanSchema("When true, list directory contents recursively. Defaults to false."),
			"include_hidden": booleanSchema("When true, include hidden files and directories in the results."),
			"max_entries":    integerSchema("Maximum number of entries to return. Values outside the supported range are clamped."),
		}, "path", "recursive", "include_hidden", "max_entries"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			resolved, err := dependencies.FilePolicy().Resolve(req.Path)
			if err != nil {
				return fileError(err), nil
			}

			recursive := false
			if req.Recursive != nil {
				recursive = *req.Recursive
			}

			limit := req.MaxEntries
			if limit <= 0 || limit > maxListEntries {
				limit = maxListEntries
			}

			entries := make([]entry, 0)
			appendEntry := func(path string, isDir bool, size int64) bool {
				rel, relErr := filepath.Rel(resolved.Root, path)
				if relErr != nil {
					return false
				}

				rel = filepath.ToSlash(rel)
				if rel == "." || rel == "" {
					return true
				}

				if !req.IncludeHidden && hiddenPath(rel) {
					return true
				}

				item := entry{Path: rel}
				if isDir {
					item.Type = "dir"
				} else {
					item.Type = "file"
					item.Size = size
				}

				entries = append(entries, item)
				return len(entries) < limit
			}

			if recursive {
				walkErr := walkDir(resolved.Absolute, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}

					if path == resolved.Absolute {
						return nil
					}

					rel, relErr := filepath.Rel(resolved.Root, path)
					if relErr == nil && !req.IncludeHidden && hiddenPath(rel) && d.IsDir() {
						return filepath.SkipDir
					}

					info, infoErr := d.Info()
					if infoErr != nil {
						return infoErr
					}

					if !appendEntry(path, d.IsDir(), info.Size()) {
						return errors.New("entry limit reached")
					}

					return nil
				})

				if walkErr != nil && walkErr.Error() != "entry limit reached" {
					return fileError(walkErr), nil
				}
			} else {
				items, err := readDir(resolved.Absolute)
				if err != nil {
					return fileError(err), nil
				}

				for _, item := range items {
					info, infoErr := item.Info()
					if infoErr != nil {
						return fileError(infoErr), nil
					}

					if !appendEntry(filepath.Join(resolved.Absolute, item.Name()), item.IsDir(), info.Size()) {
						break
					}
				}
			}

			sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

			return encodeOutput(map[string]any{
				"root":    resolved.Root,
				"path":    normalizedDisplayPath(resolved.Relative),
				"entries": entries,
			})
		}),
	}
}
