package privacy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func git(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatal("cannot inspect public repository files", err)
	}
	return output
}

func TestPublicFilesDoNotContainPersonalCheckoutPaths(t *testing.T) {
	root := repository(t)
	personalPath := regexp.MustCompile(`/(?:Users|home)/[^/\s` + "`" + `"<>]+/`)
	for _, staged := range []bool{false, true} {
		args := []string{"ls-files", "-co", "--exclude-standard", "-z"}
		if staged {
			args = []string{"diff", "--cached", "--name-only", "--diff-filter=ACMRT", "-z"}
		}
		seen := make(map[string]bool)
		for _, name := range strings.Split(string(git(t, root, args...)), "\x00") {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			var data []byte
			if staged {
				data = git(t, root, "show", ":"+name)
			} else {
				var err error
				data, err = os.ReadFile(filepath.Join(root, name))
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					t.Fatal(name, err)
				}
			}
			if personalPath.Match(data) {
				t.Errorf("personal checkout path found in %s (staged=%t)", name, staged)
			}
		}
	}
}

func TestExampleAuthenticationValuesStayBlank(t *testing.T) {
	root := repository(t)
	working, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	candidates := map[string][]byte{"worktree": working}
	for _, name := range strings.Split(string(git(t, root, "diff", "--cached", "--name-only", "--diff-filter=ACMRT", "-z")), "\x00") {
		if name == ".env.example" {
			candidates["staged"] = git(t, root, "show", ":.env.example")
		}
	}
	for label, data := range candidates {
		fields := make(map[string]string)
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if ok {
				fields[key] = strings.TrimSpace(value)
			}
		}
		for _, key := range []string{"APPBOX_CLIENT_TOKEN", "DROPBOX_APP_KEY", "MAILGUN_API_KEY", "YOURLS_SIGNATURE_SECRET"} {
			value, exists := fields[key]
			if !exists || value != "" {
				t.Errorf("example authentication field %s must exist and remain blank (%s)", key, label)
			}
		}
	}
}

func TestSensitiveLocalFilesStayIgnored(t *testing.T) {
	root := repository(t)
	for _, name := range []string{".env", ".env.production", ".env.local", ".envrc", "AGENTS.local.md", "private.key", "identity.p12", "identity.pfx", "id_rsa", "id_ed25519"} {
		command := exec.Command("git", "check-ignore", "--no-index", "-q", name)
		command.Dir = root
		if err := command.Run(); err != nil {
			t.Errorf("local sensitive file is not ignored: %s", name)
		}
	}
	tracked := strings.Split(string(git(t, root, "diff", "--cached", "--name-only", "--diff-filter=ACMRT", "-z")), "\x00")
	for _, name := range tracked {
		base := filepath.Base(name)
		if strings.HasPrefix(base, ".env") && base != ".env.example" || base == "AGENTS.local.md" || base == "id_rsa" || base == "id_ed25519" {
			t.Errorf("sensitive local file is tracked: %s", name)
		}
	}
}
