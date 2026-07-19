package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageUpload_CopiesApprovedBytesIntoPrivateImmutableFile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	source := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(source, []byte("approved"), 0o600))
	stagingRoot := filepath.Join(root, "staging")
	staged, err := stageUpload(context.Background(), source, stagingRoot, 32)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(staged.Path) })
	require.Equal(t, int64(8), staged.Size)
	require.Equal(t, sha256.Sum256([]byte("approved")), staged.Digest)
	require.Equal(t, ".txt", filepath.Ext(staged.Path))
	require.NoError(t, os.WriteFile(source, []byte("replaced"), 0o600))
	content, err := os.ReadFile(staged.Path)
	require.NoError(t, err)
	require.Equal(t, []byte("approved"), content)
	info, err := os.Stat(staged.Path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.Equal(t, ".txt", getSafeUploadExtension("report.txt"))
	require.Empty(t, getSafeUploadExtension("report.bad-extension-name"))
	require.Empty(t, getSafeUploadExtension("report.bad!"))
}

func TestStageUpload_RejectsLinksDirectoriesLimitsAndCancellation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	source := filepath.Join(root, "source.txt")
	require.NoError(t, os.WriteFile(source, []byte("content"), 0o600))
	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.Symlink(source, link))
	_, err = stageUpload(context.Background(), link, filepath.Join(root, "staging-link"), 32)
	require.Error(t, err)
	_, err = stageUpload(context.Background(), root, filepath.Join(root, "staging-dir"), 32)
	require.ErrorContains(t, err, "regular file")
	_, err = stageUpload(context.Background(), source, filepath.Join(root, "staging-size"), 1)
	require.ErrorContains(t, err, "size limit")
	hardLink := filepath.Join(root, "hard-link.txt")
	require.NoError(t, os.Link(source, hardLink))
	_, err = stageUpload(context.Background(), source, filepath.Join(root, "staging-hard-link"), 32)
	require.ErrorContains(t, err, "hard linked")
	require.NoError(t, os.Remove(hardLink))
	_, err = stageUpload(context.Background(), "relative", filepath.Join(root, "staging-relative"), 32)
	require.EqualError(t, err, "browser upload paths must be absolute")
	_, err = stageUpload(context.Background(), source, filepath.Join(root, "staging-zero"), 0)
	require.EqualError(t, err, "browser upload size limit must be greater than zero")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = stageUpload(cancelled, source, filepath.Join(root, "staging-cancelled"), 32)
	require.ErrorIs(t, err, context.Canceled)

	preexisting := filepath.Join(root, "staging-existing")
	require.NoError(t, os.Mkdir(preexisting, 0o700))
	_, err = stageUpload(context.Background(), source, preexisting, 32)
	require.ErrorIs(t, err, os.ErrExist)

	symlinkParent := filepath.Join(root, "staging-parent-link")
	require.NoError(t, os.Symlink(t.TempDir(), symlinkParent))
	_, err = stageUpload(context.Background(), source, filepath.Join(symlinkParent, "session"), 32)
	require.EqualError(t, err, "browser upload staging parent must be a regular directory")
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCopyUpload_ReportsNoProgressSizeAndWriterFailures(t *testing.T) {
	_, err := copyUpload(context.Background(), &bytes.Buffer{}, zeroReader{}, 10)
	require.ErrorContains(t, err, "made no progress")
	_, err = copyUpload(context.Background(), &bytes.Buffer{}, bytes.NewBufferString("large"), 2)
	require.EqualError(t, err, "browser upload exceeds the size limit")
	_, err = copyUpload(context.Background(), failingWriter{}, bytes.NewBufferString("data"), 10)
	require.EqualError(t, err, "write failed")
}
