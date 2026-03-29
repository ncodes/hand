package guardrails

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilesystemPolicyResolve_AllowsPathsInsideRoots(t *testing.T) {
	root := t.TempDir()
	policy := FilesystemPolicy{Roots: []string{root}}

	resolved, err := policy.Resolve("dir/file.txt")

	require.NoError(t, err)
	require.Equal(t, root, resolved.Root)
	require.Equal(t, filepath.Join(root, "dir", "file.txt"), resolved.Absolute)
	require.Equal(t, "dir/file.txt", resolved.Relative)
}

func TestFilesystemPolicyResolve_UsesRootForBlankPath(t *testing.T) {
	root := t.TempDir()
	policy := FilesystemPolicy{Roots: []string{root}}

	resolved, err := policy.Resolve("")

	require.NoError(t, err)
	require.Equal(t, root, resolved.Root)
	require.Equal(t, root, resolved.Absolute)
	require.Empty(t, resolved.Relative)
}

func TestFilesystemPolicyResolve_AllowsAbsolutePathsInsideRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dir", "file.txt")
	policy := FilesystemPolicy{Roots: []string{root}}

	resolved, err := policy.Resolve(path)

	require.NoError(t, err)
	require.Equal(t, root, resolved.Root)
	require.Equal(t, path, resolved.Absolute)
	require.Equal(t, "dir/file.txt", resolved.Relative)
}

func TestFilesystemPolicyResolve_RejectsPathsOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	policy := FilesystemPolicy{Roots: []string{root}}

	_, err := policy.Resolve(outside)

	require.EqualError(t, err, "path is outside allowed roots")
}

func TestFilesystemPolicyResolve_PrefersExistingPathAcrossRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "file.txt"), []byte("hello"), 0o644))
	policy := FilesystemPolicy{Roots: []string{rootA, rootB}}

	resolved, err := policy.Resolve("file.txt")

	require.NoError(t, err)
	require.Equal(t, rootB, resolved.Root)
	require.Equal(t, filepath.Join(rootB, "file.txt"), resolved.Absolute)
	require.Equal(t, "file.txt", resolved.Relative)
}

func TestFilesystemPolicyResolve_RejectsWhenNoRootsAreConfigured(t *testing.T) {
	_, err := (FilesystemPolicy{}).Resolve("file.txt")

	require.EqualError(t, err, "access denied")
}

func TestNormalizeRoots_DeduplicatesAndCleans(t *testing.T) {
	root := t.TempDir()
	roots := NormalizeRoots([]string{root, filepath.Join(root, "."), "", root})

	require.Len(t, roots, 1)
	require.Equal(t, root, roots[0])
}

func TestFilesystemPolicyEnsureWithin_UsesResolveResult(t *testing.T) {
	root := t.TempDir()
	policy := FilesystemPolicy{Roots: []string{root}}

	require.NoError(t, policy.EnsureWithin("file.txt"))
	require.EqualError(t, policy.EnsureWithin(filepath.Join(t.TempDir(), "secret.txt")), "path is outside allowed roots")
}

func TestReadTextFile_ReadsTextContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	content, err := ReadTextFile(path, 10)

	require.NoError(t, err)
	require.Equal(t, []byte("hello"), content)
}

func TestReadTextFile_ReturnsStatErrors(t *testing.T) {
	_, err := ReadTextFile(filepath.Join(t.TempDir(), "missing.txt"), 10)

	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestReadTextFile_RejectsDirectories(t *testing.T) {
	_, err := ReadTextFile(t.TempDir(), 10)

	require.True(t, errors.Is(err, fs.ErrInvalid))
}

func TestReadTextFile_RejectsBinaryContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.bin")
	require.NoError(t, os.WriteFile(path, []byte{0x00, 0x01}, 0o644))

	_, err := ReadTextFile(path, 10)

	require.EqualError(t, err, "file is not text")
}

func TestReadTextFile_RejectsOversizedContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	_, err := ReadTextFile(path, 3)

	require.EqualError(t, err, "file exceeds size limit")
}

func TestReadTextFile_RejectsContentThatExceedsLimitAfterRead(t *testing.T) {
	originalStatPath := statPath
	originalReadPath := readPath
	t.Cleanup(func() {
		statPath = originalStatPath
		readPath = originalReadPath
	})

	statPath = func(string) (fs.FileInfo, error) {
		return fakeFileInfo{name: "file.txt", size: 3}, nil
	}
	readPath = func(string) ([]byte, error) {
		return []byte("hello"), nil
	}

	_, err := ReadTextFile("file.txt", 4)

	require.EqualError(t, err, "file exceeds size limit")
}

func TestReadTextFile_ReturnsReadErrors(t *testing.T) {
	originalStatPath := statPath
	originalReadPath := readPath
	t.Cleanup(func() {
		statPath = originalStatPath
		readPath = originalReadPath
	})

	statPath = func(string) (fs.FileInfo, error) {
		return fakeFileInfo{name: "file.txt", size: 3}, nil
	}
	readPath = func(string) ([]byte, error) {
		return nil, errors.New("read failed")
	}

	_, err := ReadTextFile("file.txt", 10)

	require.EqualError(t, err, "read failed")
}

func TestIsBinary_HandlesEmptyAndInvalidUTF8(t *testing.T) {
	require.False(t, IsBinary(nil))
	require.True(t, IsBinary([]byte{0xff, 0xfe}))
	require.False(t, IsBinary([]byte("hello")))
}

func TestWithinRoot_DetectsEqualInsideAndOutsidePaths(t *testing.T) {
	root := t.TempDir()

	require.True(t, withinRoot(root, root))
	require.True(t, withinRoot(root, filepath.Join(root, "child.txt")))
	require.False(t, withinRoot(root, filepath.Join(root, "..", "outside.txt")))
}

type fakeFileInfo struct {
	name string
	size int64
	dir  bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }
