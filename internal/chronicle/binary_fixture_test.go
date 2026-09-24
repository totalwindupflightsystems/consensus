package chronicle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type binaryFixture struct {
	once        sync.Once
	cleanupOnce sync.Once
	build       func() (string, func(), error)
	path        string
	cleanup     func()
	err         error
}

func newBinaryFixture(build func() (string, func(), error)) *binaryFixture {
	return &binaryFixture{build: build}
}

func (f *binaryFixture) get(t testing.TB) string {
	t.Helper()
	f.once.Do(func() {
		f.path, f.cleanup, f.err = f.build()
	})
	if f.err != nil {
		t.Fatalf("build shared consensus test binary: %v", f.err)
	}
	return f.path
}

func (f *binaryFixture) close() {
	f.cleanupOnce.Do(func() {
		if f.cleanup != nil {
			f.cleanup()
		}
	})
}

var contractBinaryFixture = newBinaryFixture(buildContractBinary)

func buildContractBinary() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "consensus-contract-bin-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		cleanup()
		return "", nil, fmt.Errorf("resolve test source path")
	}
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	binPath := filepath.Join(tmpDir, "consensus")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/consensus")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("go build: %w\n%s", err, out)
	}
	return binPath, cleanup, nil
}

func TestMain(m *testing.M) {
	code := m.Run()
	contractBinaryFixture.close()
	os.Exit(code)
}

func TestBinaryFixtureBuildsOnce(t *testing.T) {
	builds := 0
	cleanups := 0
	fixture := newBinaryFixture(func() (string, func(), error) {
		builds++
		return "/tmp/consensus-test-binary", func() { cleanups++ }, nil
	})

	for i := 0; i < 17; i++ {
		if got := fixture.get(t); got != "/tmp/consensus-test-binary" {
			t.Fatalf("get() = %q, want shared binary path", got)
		}
	}
	fixture.close()
	fixture.close()

	if builds != 1 {
		t.Fatalf("builds = %d, want 1", builds)
	}
	if cleanups != 1 {
		t.Fatalf("cleanups = %d, want 1", cleanups)
	}
}
