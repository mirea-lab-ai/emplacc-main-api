// Package architecture holds FITNESS FUNCTIONS — executable guards that pin the
// hexagonal layer boundaries described in docs/adr/0001-phase3-hexagonal-fitness.md.
//
// Philosophy (per the colleague's "Fitness Functions in Software Architecture"):
// architecture that isn't measured silently decays. These tests fail the build the
// moment a NEW layer violation is introduced, while a shrinking allowlist documents
// the CURRENT legacy debt that Phase 3.1–3.5 will pay down.
//
// They run as ordinary unit tests (so `go test ./internal/...` and CI already enforce
// them) and via `make lint`.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ── helpers ────────────────────────────────────────────────────────────────

// moduleRoot walks up from the test's CWD to the directory containing go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from CWD")
		}
		dir = parent
	}
}

// goFiles returns module-root-relative paths (forward slashes) of non-test .go
// files under rel. A missing directory yields an empty slice (not a failure).
func goFiles(t *testing.T, root, rel string) []string {
	t.Helper()
	var out []string
	base := filepath.Join(root, rel)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return out
	}
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		out = append(out, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", rel, err)
	}
	return out
}

// importsOf parses just the import section of a file.
func importsOf(t *testing.T, root, relPath string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, relPath), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out
}

// rawDriverPrefixes are infrastructure drivers that must never appear above the
// adapter layer. Matched by prefix so /v9, /clause, etc. are covered.
var rawDriverPrefixes = []string{
	"gorm.io/gorm",
	"github.com/redis/go-redis",
	"github.com/minio/minio-go",
	"github.com/Nerzal/gocloak",
}

func isRawDriver(imp string) bool {
	for _, p := range rawDriverPrefixes {
		if imp == p || strings.HasPrefix(imp, p) {
			return true
		}
	}
	return false
}

// allowlist of accepted legacy layer violations (key = "relPath|import").
// Phase 3 paid down ALL of them — the map is intentionally EMPTY: the hexagon is
// fully enforced, and any new transport→repo/driver or service→driver import fails
// the build. Add an entry here only as a consciously-documented temporary exception.
var allowedLayerViolations = map[string]bool{}

func report(t *testing.T, rule string, violations []string) {
	t.Helper()
	if len(violations) == 0 {
		return
	}
	t.Errorf("%s — %d new layer violation(s):\n  %s\n\nEither fix the import or, if it is accepted legacy debt, add it to allowedLayerViolations in guards_test.go.",
		rule, len(violations), strings.Join(violations, "\n  "))
}

// ── R1: transport layer stays clean ─────────────────────────────────────────
// internal/transport/http must not import repository or raw infrastructure drivers
// (net/http is allowed — Echo handlers use its status constants).
func TestR1_TransportDoesNotImportRepositoryOrDrivers(t *testing.T) {
	root := moduleRoot(t)
	var violations []string
	for _, file := range goFiles(t, root, "internal/transport/http") {
		for _, imp := range importsOf(t, root, file) {
			bad := strings.HasPrefix(imp, "emplacc-api/internal/repository") || isRawDriver(imp)
			if !bad {
				continue
			}
			if allowedLayerViolations[file+"|"+imp] {
				continue
			}
			violations = append(violations, file+" → "+imp)
		}
	}
	report(t, "R1 (transport clean)", violations)
}

// ── R2: domain/service layer stays clean ────────────────────────────────────
// internal/service must not import transport, raw drivers, or net/http.
func TestR2_ServicesDoNotImportTransportOrDrivers(t *testing.T) {
	root := moduleRoot(t)
	var violations []string
	for _, file := range goFiles(t, root, "internal/service") {
		for _, imp := range importsOf(t, root, file) {
			bad := strings.HasPrefix(imp, "emplacc-api/internal/controller") || isRawDriver(imp) || imp == "net/http"
			if !bad {
				continue
			}
			if allowedLayerViolations[file+"|"+imp] {
				continue
			}
			violations = append(violations, file+" → "+imp)
		}
	}
	report(t, "R2 (domain clean)", violations)
}

// ── R3: context hygiene (currently 0 — hold the line) ───────────────────────
// No `if ctx == nil` defensiveness and no `ctx = context.Background()` reassignment.
func TestR3_ContextHygiene(t *testing.T) {
	root := moduleRoot(t)
	pat := regexp.MustCompile(`if\s+ctx\s*==\s*nil|ctx\s*=\s*context\.Background\(\)`)
	var violations []string
	for _, rel := range []string{"internal", "api"} {
		for _, file := range goFiles(t, root, rel) {
			b, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if loc := pat.FindIndex(b); loc != nil {
				line := 1 + strings.Count(string(b[:loc[0]]), "\n")
				violations = append(violations, file+":"+itoa(line))
			}
		}
	}
	report(t, "R3 (ctx hygiene)", violations)
}

// ── R4: DTO json tags are snake_case (FE↔BE contract) ───────────────────────
func TestR4_DTOTagsSnakeCase(t *testing.T) {
	root := moduleRoot(t)
	tag := regexp.MustCompile(`json:"([^",]+)`)
	// Live contract fields the frontend already consumes — normalize in v2 (Phase 2).
	allowedTags := map[string]bool{
		"internal/dto/response/Task.go|pageSize":      true,
		"internal/dto/response/Task.go|totalCount":    true,
		"internal/dto/response/Auth.go|userId":        true,
		"internal/dto/response/Project.go|pageSize":   true,
		"internal/dto/response/Project.go|totalCount": true,
	}
	hasUpper := regexp.MustCompile(`[A-Z]`)
	var violations []string
	for _, file := range goFiles(t, root, "internal/dto") {
		b, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range tag.FindAllStringSubmatch(string(b), -1) {
			name := m[1]
			if name == "-" || !hasUpper.MatchString(name) {
				continue
			}
			if allowedTags[file+"|"+name] {
				continue
			}
			violations = append(violations, file+" → json:\""+name+"\"")
		}
	}
	report(t, "R4 (snake_case DTO tags)", violations)
}

// ── R5 / R6: activated in later sub-phases ──────────────────────────────────
// R5 (one domain per ports file, no god service.go/repository.go) — activates in
// Phase 3.1 once internal/ports exists.
// R6 (repository List methods normalize pagination) — activates in Phase 3.4.
func TestR5_PortsOneDomainPerFile(t *testing.T) {
	if _, err := os.Stat(filepath.Join(moduleRoot(t), "internal/ports")); os.IsNotExist(err) {
		t.Skip("internal/ports not created yet — activates in Phase 3.1")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
