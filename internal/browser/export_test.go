package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteArtifactExport_PublishesPrivateFileWithoutOverwriting(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(root, "captured image λ.png")
	require.NoError(t, WriteArtifactExport(path, []byte("png")))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("png"), content)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0o077)

	err = WriteArtifactExport(path, []byte("replacement"))
	require.ErrorIs(t, err, os.ErrExist)
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("png"), content)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestWriteArtifactExport_FallsBackToExclusiveCreateOnlyForUnsupportedLinks(t *testing.T) {
	originalPublish := publishArtifactExport
	originalCheck := checkArtifactExportLinkUnsupported
	originalWrite := writeArtifactExportContent
	t.Cleanup(func() {
		publishArtifactExport = originalPublish
		checkArtifactExportLinkUnsupported = originalCheck
		writeArtifactExportContent = originalWrite
	})
	linkErr := errors.New("link unsupported")
	publishArtifactExport = func(string, string) error { return linkErr }
	checkArtifactExportLinkUnsupported = func(err error) bool { return errors.Is(err, linkErr) }
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(root, "fallback.pdf")

	require.NoError(t, WriteArtifactExport(path, []byte("pdf")))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("pdf"), content)
	err = WriteArtifactExport(path, []byte("replacement"))
	require.ErrorIs(t, err, os.ErrExist)
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("pdf"), content)

	checkArtifactExportLinkUnsupported = func(error) bool { return false }
	other := filepath.Join(root, "not-created.pdf")
	err = WriteArtifactExport(other, []byte("pdf"))
	require.ErrorIs(t, err, linkErr)
	require.NoFileExists(t, other)

	writeCalls := 0
	writeArtifactExportContent = func(file *os.File, data []byte) error {
		writeCalls++
		if writeCalls == 1 {
			return originalWrite(file, data)
		}
		_, _ = file.Write(data[:1])
		return errors.New("write failed")
	}
	checkArtifactExportLinkUnsupported = func(error) bool { return true }
	failed := filepath.Join(root, "fallback-failed.pdf")
	err = WriteArtifactExport(failed, []byte("pdf"))
	require.EqualError(t, err, "write failed")
	require.NoFileExists(t, failed)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".part")
	}
}

func TestWriteArtifactExport_RemovesPartialFileWhenTemporaryWriteFails(t *testing.T) {
	originalWrite := writeArtifactExportContent
	t.Cleanup(func() { writeArtifactExportContent = originalWrite })
	writeArtifactExportContent = func(file *os.File, data []byte) error {
		_, _ = file.Write(data[:1])
		return errors.New("sync failed")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	destination := filepath.Join(root, "failed.png")

	err = WriteArtifactExport(destination, []byte("png"))
	require.EqualError(t, err, "sync failed")
	require.NoFileExists(t, destination)
	entries, readErr := os.ReadDir(root)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestWriteArtifactExport_KeepsPublishedDestinationWhenTemporaryCleanupFails(t *testing.T) {
	originalPublish := publishArtifactExport
	originalCheck := checkArtifactExportLinkUnsupported
	originalRemove := removeArtifactExportTemporary
	t.Cleanup(func() {
		publishArtifactExport = originalPublish
		checkArtifactExportLinkUnsupported = originalCheck
		removeArtifactExportTemporary = originalRemove
	})
	cleanupErr := errors.New("temporary cleanup failed")
	removeArtifactExportTemporary = func(string) error { return cleanupErr }

	for _, test := range []struct {
		name     string
		fallback bool
	}{
		{name: "hard link"},
		{name: "exclusive fallback", fallback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			publishArtifactExport = originalPublish
			checkArtifactExportLinkUnsupported = originalCheck
			if test.fallback {
				unsupportedErr := errors.New("hard links unsupported")
				publishArtifactExport = func(string, string) error { return unsupportedErr }
				checkArtifactExportLinkUnsupported = func(err error) bool { return errors.Is(err, unsupportedErr) }
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			destination := filepath.Join(root, "published.bin")

			require.NoError(t, WriteArtifactExport(destination, []byte("artifact")))
			content, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, []byte("artifact"), content)
		})
	}
}

func TestWriteArtifactExport_RejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symbolic links unavailable")
	}

	err := WriteArtifactExport(filepath.Join(link, "artifact.bin"), []byte("data"))
	require.EqualError(t, err, "browser artifact export path must not traverse a symbolic link or junction")
	require.NoFileExists(t, filepath.Join(target, "artifact.bin"))

	resolved, err := ResolveArtifactExportPath(filepath.Join(link, "artifact.bin"))
	require.NoError(t, err)
	target, err = filepath.EvalSymlinks(target)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(target, "artifact.bin"), resolved)
}
