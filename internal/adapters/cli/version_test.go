package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/buildinfo"
)

func TestVersionTextOutput(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"version"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	want := strings.Join([]string{
		buildinfo.Product + " " + buildinfo.Version,
		"goVersion: " + runtime.Version(),
		"commit: " + buildinfo.Commit,
		"dirty: false",
		"",
	}, "\n")
	if stdout.String() != want {
		t.Fatalf("text mismatch\n got: %q\nwant: %q", stdout.String(), want)
	}
	assertNoTimestamp(t, stdout.Bytes())
}

func TestVersionJSONOutputMatchesSchemaAndGoldenShape(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"version", "--output", "json"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	raw := stdout.Bytes()
	assertNoTimestamp(t, raw)
	assertSingleJSONDocument(t, raw)
	assertNoForeignURL(t, raw)
	validateAgainstCheckedInSchema(t, raw, "version.schema.json")

	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env["schemaVersion"] != cli.SchemaVersion {
		t.Fatalf("schemaVersion=%v", env["schemaVersion"])
	}
	if env["command"] != "version" {
		t.Fatalf("command=%v", env["command"])
	}
	if env["ok"] != true {
		t.Fatalf("ok=%v", env["ok"])
	}
	diags, ok := env["diagnostics"].([]any)
	if !ok || len(diags) != 0 {
		t.Fatalf("diagnostics=%v", env["diagnostics"])
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("result type %T", env["result"])
	}
	wantKeys := []string{"product", "version", "goVersion", "commit", "dirty"}
	if len(result) != len(wantKeys) {
		t.Fatalf("result keys=%v", result)
	}
	for _, k := range wantKeys {
		if _, exists := result[k]; !exists {
			t.Fatalf("missing result.%s", k)
		}
	}
	if result["product"] != buildinfo.Product {
		t.Fatalf("product=%v", result["product"])
	}
	if result["version"] != buildinfo.Version {
		t.Fatalf("version=%v", result["version"])
	}
	if result["goVersion"] != runtime.Version() {
		t.Fatalf("goVersion=%v", result["goVersion"])
	}
	if result["commit"] != buildinfo.Commit {
		t.Fatalf("commit=%v", result["commit"])
	}
	if result["dirty"] != false {
		t.Fatalf("dirty=%v", result["dirty"])
	}

	goldenPath := filepath.Join(repoRoot(t), "testdata", "golden", "version", "version.json")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatalf("golden unmarshal: %v", err)
	}
	wantResult := want["result"].(map[string]any)
	wantResult["goVersion"] = runtime.Version()
	if !jsonDeepEqual(env, want) {
		gotNorm, _ := json.Marshal(env)
		wantNorm, _ := json.Marshal(want)
		t.Fatalf("golden mismatch\n got: %s\nwant: %s", gotNorm, wantNorm)
	}
}

func jsonDeepEqual(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	var an, bn any
	if err := json.Unmarshal(ab, &an); err != nil {
		return false
	}
	if err := json.Unmarshal(bb, &bn); err != nil {
		return false
	}
	return jsonEqual(an, bn)
}

func jsonEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !jsonEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func TestVersionRejectsUnknownOutput(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"version", "--output", "yaml"}, stdout, stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(stderr.String(), "unsupported --output") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func assertSingleJSONDocument(t *testing.T, raw []byte) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	var first any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("first decode: %v", err)
	}
	var second any
	if err := dec.Decode(&second); err != nil {
		return
	}
	t.Fatalf("expected exactly one JSON document, found another: %v", second)
}

func assertNoTimestamp(t *testing.T, raw []byte) {
	t.Helper()
	lower := strings.ToLower(string(raw))
	for _, needle := range []string{
		"timestamp",
		"builtat",
		"buildtime",
		"date:",
		time.Now().UTC().Format("2006-01-02"),
	} {
		if strings.Contains(lower, needle) {
			t.Fatalf("output unexpectedly contains %q: %s", needle, raw)
		}
	}
}

func assertNoForeignURL(t *testing.T, raw []byte) {
	t.Helper()
	lower := strings.ToLower(string(raw))
	for _, needle := range []string{"http://", "https://", "github.com/", "golang.org/"} {
		if strings.Contains(lower, needle) {
			t.Fatalf("contains foreign address marker %q: %s", needle, raw)
		}
	}
}

// validateAgainstCheckedInSchema checks the instance against the local schema
// document using the Go standard library only (Draft 2020-12 subset).
func validateAgainstCheckedInSchema(t *testing.T, raw []byte, schemaName string) {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "schemas", "cli", "v1")
	schemaPath := filepath.Join(dir, schemaName)
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	assertNoForeignURL(t, schemaBytes)

	registry, err := loadCLISchemaRegistry(dir)
	if err != nil {
		t.Fatalf("schema registry: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		t.Fatalf("instance: %v", err)
	}
	if err := validateSchema(schema, instance, registry); err != nil {
		t.Fatalf("schema validation: %v\ninstance=%s", err, raw)
	}
}

func loadCLISchemaRegistry(dir string) (map[string]map[string]any, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	reg := make(map[string]map[string]any)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if id, ok := doc["$id"].(string); ok && id != "" {
			reg[id] = doc
		}
	}
	return reg, nil
}

func validateSchema(schema map[string]any, val any, reg map[string]map[string]any) error {
	if ref, ok := schema["$ref"].(string); ok {
		target, ok := reg[ref]
		if !ok {
			return fmt.Errorf("unresolved $ref %q", ref)
		}
		return validateSchema(target, val, reg)
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		var matched int
		var last error
		for i, raw := range oneOf {
			alt, _ := raw.(map[string]any)
			if err := validateSchema(alt, val, reg); err != nil {
				last = fmt.Errorf("oneOf[%d]: %w", i, err)
				continue
			}
			matched++
		}
		if matched != 1 {
			if last == nil {
				last = fmt.Errorf("oneOf matched %d alternatives", matched)
			}
			return fmt.Errorf("oneOf: matched %d: %w", matched, last)
		}
		return nil
	}
	if c, ok := schema["const"]; ok {
		if !jsonEqual(c, val) {
			return fmt.Errorf("const mismatch: got %#v want %#v", val, c)
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if jsonEqual(e, val) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value %#v not in enum", val)
		}
	}
	switch schema["type"] {
	case "null":
		if val != nil {
			return fmt.Errorf("want null, got %T", val)
		}
	case "string":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("want string, got %T", val)
		}
		if min, ok := schema["minLength"].(float64); ok && float64(len(s)) < min {
			return fmt.Errorf("string shorter than minLength")
		}
		if pat, ok := schema["pattern"].(string); ok && pat != "" {
			re, err := compilePattern(pat)
			if err != nil {
				return err
			}
			if !re.MatchString(s) {
				return fmt.Errorf("string %q does not match pattern %q", s, pat)
			}
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("want bool, got %T", val)
		}
	case "integer":
		n, ok := val.(float64)
		if !ok {
			return fmt.Errorf("want integer, got %T", val)
		}
		if n != float64(int64(n)) {
			return fmt.Errorf("want integer, got %v", n)
		}
		if min, ok := schema["minimum"].(float64); ok && n < min {
			return fmt.Errorf("integer below minimum")
		}
	case "array":
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("want array, got %T", val)
		}
		if max, ok := schema["maxItems"].(float64); ok && float64(len(arr)) > max {
			return fmt.Errorf("array longer than maxItems")
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				if err := validateSchema(items, item, reg); err != nil {
					return fmt.Errorf("[%d]: %w", i, err)
				}
			}
		}
	case "object":
		obj, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("want object, got %T", val)
		}
		return validateObject(schema, obj, reg)
	case nil, "":
		// typeless schema (e.g. envelope result) — structural keywords only
	default:
		return fmt.Errorf("unsupported schema type %v", schema["type"])
	}
	return nil
}

func validateObject(schema map[string]any, instance map[string]any, reg map[string]map[string]any) error {
	props, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	for _, r := range required {
		key, _ := r.(string)
		if _, ok := instance[key]; !ok {
			return fmt.Errorf("missing required %q", key)
		}
	}
	if addl, ok := schema["additionalProperties"].(bool); ok && !addl {
		for k := range instance {
			if _, ok := props[k]; !ok {
				return fmt.Errorf("unexpected property %q", k)
			}
		}
	}
	for key, rawProp := range props {
		val, ok := instance[key]
		if !ok {
			continue
		}
		prop, _ := rawProp.(map[string]any)
		if err := validateSchema(prop, val, reg); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func compilePattern(pat string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("pattern %q: %w", pat, err)
	}
	return re, nil
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
