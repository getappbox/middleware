package runner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../Scripts/test.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "Scripts/test.sh")
	if err := os.WriteFile(path, script, 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "tools with spaces")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	goScript := `#!/bin/bash
if [ "$1" = "version" ]; then
    echo 'go version go1.27.3 test/test'
    exit 0
fi
printf 'args=%s\ncwd=%s\ncompiler=%s\n' "$*" "$PWD" "$GO_BIN" >> "$FAKE_LOG"
exit "${FAKE_GO_STATUS:-0}"
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(goScript), 0700); err != nil {
		t.Fatal(err)
	}
	dockerScript := `#!/bin/bash
echo "$*" >> "$DOCKER_LOG"
exit 1
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(dockerScript), 0700); err != nil {
		t.Fatal(err)
	}
	return root, path, bin
}

func invoke(t *testing.T, root, script, bin, override, status string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("/bin/bash", append([]string{script}, args...)...)
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "GO_BIN=" + override, "FAKE_LOG=" + filepath.Join(root, "go.log"), "DOCKER_LOG=" + filepath.Join(root, "docker.log"), "FAKE_GO_STATUS=" + status, "TMPDIR=" + root}
	return cmd.CombinedOutput()
}

func TestCompilerResolutionAndForwarding(t *testing.T) {
	for _, selection := range []string{"path", "absolute-with-spaces", "relative-with-spaces", "command-name"} {
		t.Run(selection, func(t *testing.T) {
			root, script, bin := fixture(t)
			override := ""
			switch selection {
			case "absolute-with-spaces":
				override = filepath.Join(bin, "go")
			case "relative-with-spaces":
				override = "./tools with spaces/go"
			case "command-name":
				override = "go"
			}
			output, err := invoke(t, root, script, bin, override, "0", "unit")
			if err != nil {
				t.Fatalf("%v: %s", err, output)
			}
			log, err := os.ReadFile(filepath.Join(root, "go.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{"args=test -race -count=1 ./...", "cwd=" + root, "compiler=" + filepath.Join(bin, "go")} {
				if !strings.Contains(string(log), expected) {
					t.Fatalf("missing %q in %s", expected, log)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "docker.log")); !os.IsNotExist(err) {
				t.Fatal("unit run contacted Docker")
			}
		})
	}
}

func TestMissingOrFailingCompilerStopsBeforeDocker(t *testing.T) {
	for _, selection := range []string{"missing-override", "non-executable", "failed-tests"} {
		t.Run(selection, func(t *testing.T) {
			root, script, bin := fixture(t)
			override, status := filepath.Join(bin, "go"), "0"
			switch selection {
			case "missing-override":
				override = filepath.Join(root, "missing-go")
			case "non-executable":
				override = filepath.Join(root, "not-executable")
				if err := os.WriteFile(override, []byte("not executable"), 0600); err != nil {
					t.Fatal(err)
				}
			case "failed-tests":
				status = "1"
			}
			output, err := invoke(t, root, script, bin, override, status, "all")
			if err == nil {
				t.Fatalf("failure became success: %s", output)
			}
			if selection != "failed-tests" && !strings.Contains(string(output), "GO_BIN does not identify an executable") {
				t.Fatal(string(output))
			}
			if _, err := os.Stat(filepath.Join(root, "docker.log")); !os.IsNotExist(err) {
				t.Fatal("failed native suite contacted Docker")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "tmp.") {
					t.Fatal("prerequisite failure leaked temporary response files")
				}
			}
		})
	}
}

func TestHelpAndContainerOnlyDoNotRequireGo(t *testing.T) {
	for _, mode := range []string{"--help", "e2e"} {
		t.Run(mode, func(t *testing.T) {
			root, script, bin := fixture(t)
			output, err := invoke(t, root, script, bin, filepath.Join(root, "missing-go"), "0", mode)
			if mode == "--help" {
				if err != nil || !strings.Contains(string(output), "Usage:") {
					t.Fatalf("help: %v: %s", err, output)
				}
			} else {
				if err == nil || strings.Contains(string(output), "GO_BIN") {
					t.Fatalf("container-only run checked compiler: %v: %s", err, output)
				}
				log, readErr := os.ReadFile(filepath.Join(root, "docker.log"))
				if readErr != nil || !strings.Contains(string(log), "info") {
					t.Fatalf("expected Docker prerequisite check: %s: %v", log, readErr)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "go.log")); !os.IsNotExist(err) {
				t.Fatal("help/container run contacted Go")
			}
		})
	}
}
