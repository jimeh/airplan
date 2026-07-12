package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jimeh/airplan/airplan"
)

func TestConfigSchemaCmd(t *testing.T) {
	want, err := airplan.ConfigSchema()
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"schema"})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("output differs from ConfigSchema()")
	}
}

func TestConfigSchemaCmdRejectsArguments(t *testing.T) {
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"schema", "extra"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown command") &&
		!strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("error = %v, want argument rejection", err)
	}
}

func TestConfigShowTableReportsSourcesAndRedactsCredentials(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "access-sentinel")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "secret-sentinel")
	path := writeConfigShowFixture(t)

	stdout, stderr, err := executeConfigCommand(t,
		"show", "--config", path, "--bucket", "flag-bucket",
		"--no-source=false")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"CONFIG FILE", path, "--config",
		"PROFILE", "work", "default_profile",
		"CREDENTIALS", "explicit access keys",
		"bucket", "flag-bucket", "--bucket",
		"endpoint", "https://work.example.com", "profiles.work.endpoint",
		"no_source", "false", "--no-source",
		"access_key_id", "<set>", "AIRPLAN_ACCESS_KEY_ID",
		"secret_access_key", "AIRPLAN_SECRET_ACCESS_KEY",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
	for _, secret := range []string{
		"access-sentinel", "secret-sentinel", "file-access", "file-secret",
	} {
		if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
			t.Fatalf("credential %q leaked: stdout=%q stderr=%q",
				secret, stdout, stderr)
		}
	}
}

func TestConfigShowJSONIsStructuredAndRedacted(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "json-access-sentinel")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "json-secret-sentinel")
	path := writeConfigShowFixture(t)

	stdout, stderr, err := executeConfigCommand(t,
		"show", "--json", "--config", path)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stdout, "json-access-sentinel") ||
		strings.Contains(stdout, "json-secret-sentinel") {
		t.Fatalf("JSON leaked credentials: %s", stdout)
	}
	if strings.Contains(stdout, `"sources"`) {
		t.Fatalf("JSON unexpectedly exposed shadowed source chains: %s", stdout)
	}
	var output configShowJSON
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
	}
	if output.ConfigFile.Path != path || !output.ConfigFile.Exists ||
		output.ConfigFile.Source.Name != "--config" {
		t.Fatalf("config file = %+v", output.ConfigFile)
	}
	if output.Profile.Name == nil || *output.Profile.Name != "work" ||
		output.Profile.Root {
		t.Fatalf("profile = %+v", output.Profile)
	}
	secret := output.Fields["secret_access_key"]
	if secret.Value != nil || !secret.Set || !secret.Sensitive ||
		secret.Source == nil ||
		secret.Source.Name != "AIRPLAN_SECRET_ACCESS_KEY" {
		t.Fatalf("secret field = %+v", secret)
	}
	if _, ok := output.Fields["endpoint"]; !ok {
		t.Fatalf("endpoint missing: %+v", output.Fields)
	}
}

func TestConfigShowWorksWithIncompleteRootConfig(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stdout, stderr, err := executeConfigCommand(t, "show")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	for _, want := range []string{
		"CONFIG FILE", "(not found)",
		"PROFILE", "<root>", "no named profiles",
		"CREDENTIALS", "standard AWS credential chain",
		"endpoint", "<unset>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
	var endpointRow string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "endpoint ") {
			endpointRow = line
			break
		}
	}
	if strings.Count(endpointRow, "<unset>") != 2 {
		t.Fatalf("unset endpoint row = %q, want unset value and source",
			endpointRow)
	}
}

func TestConfigShowReportsPartialCredentialsWithoutValidation(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "partial-access-sentinel")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stdout, stderr, err := executeConfigCommand(t, "show")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "partial explicit configuration") ||
		strings.Contains(stdout, "partial-access-sentinel") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func writeConfigShowFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := `
bucket = "root-bucket"
default_profile = "work"
access_key_id = "file-access"
secret_access_key = "file-secret"
no_source = true

[profiles.work]
endpoint = "https://work.example.com"
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func executeConfigCommand(
	t *testing.T, args ...string,
) (string, string, error) {
	t.Helper()
	cmd := newConfigCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
