// check-migrations-rls scans supabase/migrations/*.sql and enforces the
// rule documented in supabase/migrations/GUIDELINES.md: every table that
// declares a tenant_id column must have RLS enabled and at least one
// policy created — either explicitly in the same migration, or via a
// later migration referencing the table by schema.table, or via a
// dynamic DO block whose schema list and tenant_id filter cover it.
//
// P2/M1 (revisão de permissões de 2026-09-19) acrescentou uma segunda
// classe: as tabelas de IDENTIDADE/plumbing, que são multi-tenant mas não
// têm coluna tenant_id (o escopo vem de account_id/company_id ou de nada —
// dedup do relay). Elas escapavam do check exatamente por isso: oito
// tabelas ficaram sem RLS, incluindo toca_auth.accounts. A lista
// identityTables abaixo é explícita e cada entrada diz por que a política é
// ou não exigida.
//
// Usage:
//
//	go run ./scripts/check-migrations-rls           # default paths
//	go run ./scripts/check-migrations-rls -dir=path -allowlist=path
//
// Exit codes:
//
//	0 — every multi-tenant table is covered
//	1 — at least one table is missing RLS or a policy
//	2 — usage / IO error
//
// The intent is to catch the failure mode that already burned us: a new
// table lands with a tenant_id column but no ENABLE ROW LEVEL SECURITY
// in the same migration, and quietly leaks across tenants until
// somebody notices.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type table struct {
	Schema string
	Name   string
	File   string
	Offset int
}

func (t table) FullName() string { return t.Schema + "." + t.Name }

// createTableRe matches "CREATE TABLE [IF NOT EXISTS] schema.table".
// We capture schema and table; the body is extracted separately by
// scanning forward to the closing paren of the column list. Bare table
// names (no schema) are accepted and bucketed under schema "public" to
// match PostgreSQL's default.
var createTableRe = regexp.MustCompile(`(?i)create\s+table\s+(?:if\s+not\s+exists\s+)?(?:([a-z_][a-z0-9_]*)\.)?([a-z_][a-z0-9_]*)\s*\(`)

// hasTenantIDColumnRe scans the column list for a tenant_id column.
// We allow any whitespace/type — the only thing we care about is that
// the table is in the tenant-scoped club.
var hasTenantIDColumnRe = regexp.MustCompile(`(?i)\btenant_id\b\s+[a-z]`)

// enableRLSRe matches a literal "ALTER TABLE [IF EXISTS] schema.table
// ... ENABLE ROW LEVEL SECURITY" — the placeholder %I.%I form (used in
// DO blocks) is handled separately by dynamicCoversTable.
var enableRLSRe = regexp.MustCompile(`(?is)alter\s+table\s+(?:if\s+exists\s+)?([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)[^;]*?enable\s+row\s+level\s+security`)

// createPolicyRe matches "CREATE POLICY name ON schema.table".
var createPolicyRe = regexp.MustCompile(`(?is)create\s+policy\s+[a-z_][a-z0-9_]*\s+on\s+([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)`)

// dropTableRe tracks tables that no longer exist at the end of the migration
// lineage. A dropped tenant table does not need a policy in the final schema.
var dropTableRe = regexp.MustCompile(`(?i)drop\s+table\s+(?:if\s+exists\s+)?(?:([a-z_][a-z0-9_]*)\.)?([a-z_][a-z0-9_]*)`)

// dynamicBlockRe pulls out DO $$ ... END $$ blocks so we can inspect
// them. Group 1 is the block body.
var dynamicBlockRe = regexp.MustCompile(`(?is)\bdo\s+\$\$(.*?)\bend\s*(?:if\s+)?\$\$`)

// dynamicSchemaListRe extracts the schema list from a SQL "IN (...)"
// clause inside a DO block. Used to know which schemas the dynamic RLS
// loop covers.
var dynamicSchemaListRe = regexp.MustCompile(`(?is)\bnspname\s+in\s*\(([^)]+)\)`)

func main() {
	dir := flag.String("dir", "supabase/migrations", "directory containing .sql migrations")
	allowlistPath := flag.String("allowlist", "", "path to allowlist (default: <dir>/.no-rls-allowlist.txt)")
	verbose := flag.Bool("v", false, "print per-table status, not just failures")
	flag.Parse()

	if *allowlistPath == "" {
		*allowlistPath = filepath.Join(*dir, ".no-rls-allowlist.txt")
	}

	files, err := listSQLFiles(*dir)
	if err != nil {
		fail(2, "list migrations: %v", err)
	}
	if len(files) == 0 {
		fail(2, "no .sql files under %s", *dir)
	}

	allow, err := loadAllowlist(*allowlistPath)
	if err != nil {
		fail(2, "load allowlist %s: %v", *allowlistPath, err)
	}

	declaredTables, err := extractTenantTables(files)
	if err != nil {
		fail(2, "parse migrations: %v", err)
	}

	staticRLS, err := indexMatches(files, enableRLSRe)
	if err != nil {
		fail(2, "index RLS statements: %v", err)
	}
	staticPolicies, err := indexMatches(files, createPolicyRe)
	if err != nil {
		fail(2, "index policies: %v", err)
	}
	dropPositions, err := indexMatches(files, dropTableRe)
	if err != nil {
		fail(2, "index dropped tables: %v", err)
	}
	tables, droppedTables := resolveTenantTableLifecycles(declaredTables, dropPositions)
	dynamicSchemas, err := dynamicallyCoveredSchemas(files)
	if err != nil {
		fail(2, "index dynamic RLS blocks: %v", err)
	}
	// Todas as tabelas declaradas (não só as com tenant_id): necessário para a
	// classe de identidade do P2/M1, que é multi-tenant sem coluna tenant_id.
	allTables, err := indexMatches(files, createTableRe)
	if err != nil {
		fail(2, "index declared tables: %v", err)
	}

	var failures []string
	checked := 0
	allowlisted := 0
	dropped := len(droppedTables)

	if *verbose {
		for _, t := range droppedTables {
			fmt.Printf("DROP  %s (last declared in %s)\n", t.FullName(), filepath.Base(t.File))
		}
	}

	for _, t := range tables {
		full := t.FullName()
		if allow[full] {
			allowlisted++
			if *verbose {
				fmt.Printf("ALLOW %s (declared in %s)\n", full, filepath.Base(t.File))
			}
			continue
		}
		checked++

		hasRLS := coveredAfter(t, staticRLS[full]) || coveredAfter(t, dynamicSchemas[t.Schema])
		hasPolicy := coveredAfter(t, staticPolicies[full]) || coveredAfter(t, dynamicSchemas[t.Schema])

		switch {
		case !hasRLS && !hasPolicy:
			failures = append(failures,
				fmt.Sprintf("%s — missing ENABLE ROW LEVEL SECURITY and CREATE POLICY (declared in %s)",
					full, filepath.Base(t.File)))
		case !hasRLS:
			failures = append(failures,
				fmt.Sprintf("%s — has policy but no ENABLE ROW LEVEL SECURITY (declared in %s)",
					full, filepath.Base(t.File)))
		case !hasPolicy:
			failures = append(failures,
				fmt.Sprintf("%s — RLS enabled but no CREATE POLICY found (declared in %s)",
					full, filepath.Base(t.File)))
		default:
			if *verbose {
				fmt.Printf("OK    %s (declared in %s)\n", full, filepath.Base(t.File))
			}
		}
	}

	failures = append(failures, checkIdentityTables(allTables, staticRLS, staticPolicies, dynamicSchemas)...)

	// Stale-allowlist check — drift here is how the allowlist quietly
	// expands its blast radius. If an allowlisted table no longer
	// appears in the migration set, the entry should be removed.
	declared := map[string]bool{}
	for _, t := range declaredTables {
		declared[t.FullName()] = true
	}
	for entry := range allow {
		if !declared[entry] {
			failures = append(failures,
				fmt.Sprintf("%s — allowlisted but not declared in any migration (drop the entry?)", entry))
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		fmt.Fprintln(os.Stderr, "RLS migration check failed:")
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, "  - "+f)
		}
		fmt.Fprintf(os.Stderr, "\nSee supabase/migrations/GUIDELINES.md.\n")
		os.Exit(1)
	}

	fmt.Printf("OK: %d multi-tenant tables checked, %d identity tables checked, %d allowlisted, %d dropped, %d migration files scanned.\n",
		checked, identityChecked, allowlisted, dropped, len(files))
}

// identityTable é uma tabela multi-tenant SEM coluna tenant_id.
type identityTable struct {
	policyRequired bool
	reason         string
}

// identityTables documenta as tabelas de identidade/plumbing que precisam de
// RLS (achado M1). Tabelas onde o pool de aplicação lê/escreve exigem política
// vinculada ao JWT; as que só o relay/serviço toca ficam deny-by-default.
var identityTables = map[string]identityTable{
	"toca_auth.accounts":         {true, "conta: id = sub do JWT"},
	"toca_auth.credentials":      {true, "credencial de login: account_id"},
	"toca_auth.user_preferences": {true, "preferências: account_id"},
	"toca_auth.invitations":      {true, "convite: company_id = tenant do JWT"},
	"toca_auth.groups":           {true, "grupo: company_id = tenant do JWT"},
	"core.partnerships":          {true, "parceria entre dois tenants: requester_id/target_id"},
	"toca_auth.password_resets":  {false, "fluxo de token local removido no P1/A14; deny-by-default"},
	"events.processed_events":    {false, "dedup do relay de eventos; acessado só pelo pool de serviço"},
}

// checkIdentityTables valida a segunda classe de tabelas (P2/M1): RLS ligado em
// todas e política presente onde o pool de aplicação consulta. Só falha quando
// a tabela é declarada em alguma migration — o check é conservador com tabelas
// criadas em runtime.
func checkIdentityTables(declared map[string][]sourcePosition, staticRLS, staticPolicies, dynamicSchemas map[string][]sourcePosition) []string {
	var failures []string
	checked := 0
	for name, spec := range identityTables {
		positions := declared[name]
		if len(positions) == 0 {
			continue // criada em runtime (schema_bootstrap) ou renomeada
		}
		// Última declaração: a RLS precisa vir depois dela.
		last := positions[len(positions)-1]
		schema := name[:strings.Index(name, ".")]
		decl := table{Schema: schema, Name: name[strings.Index(name, ".")+1:], File: last.File, Offset: last.Offset}
		checked++
		hasRLS := coveredAfter(decl, staticRLS[name]) || coveredAfter(decl, dynamicSchemas[schema])
		hasPolicy := coveredAfter(decl, staticPolicies[name]) || coveredAfter(decl, dynamicSchemas[schema])
		switch {
		case !hasRLS:
			failures = append(failures, fmt.Sprintf(
				"%s — tabela de identidade sem ENABLE ROW LEVEL SECURITY (achado M1: %s)", name, spec.reason))
		case spec.policyRequired && !hasPolicy:
			failures = append(failures, fmt.Sprintf(
				"%s — tabela de identidade consultada pelo pool de aplicação sem CREATE POLICY (achado M1: %s)", name, spec.reason))
		}
	}
	identityChecked = checked
	return failures
}

// identityChecked é o contador exposto no resumo final.
var identityChecked int

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-migrations-rls: "+format+"\n", args...)
	os.Exit(code)
}

func listSQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func loadAllowlist(path string) (map[string]bool, error) {
	out := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		// Strip inline comments first, then trim.
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, ".") {
			return nil, fmt.Errorf("entry %q must be schema.table", line)
		}
		out[strings.ToLower(line)] = true
	}
	return out, nil
}

// extractTenantTables walks each migration in order, finds every
// CREATE TABLE … (…) block, and returns the ones that contain a
// tenant_id column. Every declaration is retained so lifecycle resolution can
// distinguish a harmless repeated CREATE IF NOT EXISTS from DROP -> CREATE,
// which creates a new table that needs fresh RLS and policies.
func extractTenantTables(files []string) ([]table, error) {
	var out []table
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		body := string(data)
		for _, idx := range createTableRe.FindAllStringSubmatchIndex(body, -1) {
			schemaStart, schemaEnd := idx[2], idx[3]
			nameStart, nameEnd := idx[4], idx[5]
			matchEnd := idx[1]

			schema := "public"
			if schemaStart >= 0 {
				schema = strings.ToLower(body[schemaStart:schemaEnd])
			}
			name := strings.ToLower(body[nameStart:nameEnd])

			columns, ok := captureBalancedParens(body, matchEnd-1)
			if !ok {
				continue
			}
			if !hasTenantIDColumnRe.MatchString(columns) {
				continue
			}
			out = append(out, table{Schema: schema, Name: name, File: f, Offset: idx[0]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FullName() != out[j].FullName() {
			return out[i].FullName() < out[j].FullName()
		}
		return positionBefore(tablePosition(out[i]), tablePosition(out[j]))
	})
	return out, nil
}

// captureBalancedParens returns the content between the opening paren
// at start and its matching closing paren. start should point at the
// '('. It naively skips over '$$ ... $$' dollar-quoted blocks; that's
// enough for our migrations which don't nest CREATE TABLE inside
// other dollar-quoted code.
func captureBalancedParens(body string, start int) (string, bool) {
	if start >= len(body) || body[start] != '(' {
		return "", false
	}
	depth := 1
	i := start + 1
	for i < len(body) {
		// Skip $$ … $$ blocks wholesale.
		if i+1 < len(body) && body[i] == '$' && body[i+1] == '$' {
			j := strings.Index(body[i+2:], "$$")
			if j < 0 {
				return "", false
			}
			i = i + 2 + j + 2
			continue
		}
		// Skip single-quoted strings (with doubled-quote escape).
		if body[i] == '\'' {
			i++
			for i < len(body) {
				if body[i] == '\'' {
					if i+1 < len(body) && body[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		// Skip -- line comments.
		if i+1 < len(body) && body[i] == '-' && body[i+1] == '-' {
			j := strings.IndexByte(body[i:], '\n')
			if j < 0 {
				return "", false
			}
			i += j + 1
			continue
		}
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return body[start+1 : i], true
			}
		}
		i++
	}
	return "", false
}

type sourcePosition struct {
	File   string
	Offset int
}

func tablePosition(t table) sourcePosition {
	return sourcePosition{File: t.File, Offset: t.Offset}
}

func positionBefore(left, right sourcePosition) bool {
	return left.File < right.File || (left.File == right.File && left.Offset < right.Offset)
}

func positionAfter(left, right sourcePosition) bool {
	return positionBefore(right, left)
}

// resolveTenantTableLifecycles returns one declaration per table in the final
// schema. Repeated CREATE IF NOT EXISTS statements in the same lifecycle keep
// the first declaration because they do not reset RLS. A DROP starts a new
// lifecycle, so the first later CREATE must receive fresh RLS and policies.
// Tables whose final lifecycle event is DROP are returned separately.
func resolveTenantTableLifecycles(
	declarations []table,
	dropPositions map[string][]sourcePosition,
) (active []table, dropped []table) {
	byName := map[string][]table{}
	for _, declaration := range declarations {
		full := declaration.FullName()
		byName[full] = append(byName[full], declaration)
	}

	for full, creates := range byName {
		sort.Slice(creates, func(i, j int) bool {
			return positionBefore(tablePosition(creates[i]), tablePosition(creates[j]))
		})

		var lastDrop sourcePosition
		hasDrop := false
		for _, drop := range dropPositions[full] {
			if !hasDrop || positionAfter(drop, lastDrop) {
				lastDrop = drop
				hasDrop = true
			}
		}

		lastCreate := creates[len(creates)-1]
		if hasDrop && positionAfter(lastDrop, tablePosition(lastCreate)) {
			dropped = append(dropped, lastCreate)
			continue
		}

		lifecycleStart := creates[0]
		if hasDrop {
			for _, create := range creates {
				if positionAfter(tablePosition(create), lastDrop) {
					lifecycleStart = create
					break
				}
			}
		}
		active = append(active, lifecycleStart)
	}

	sort.Slice(active, func(i, j int) bool { return active[i].FullName() < active[j].FullName() })
	sort.Slice(dropped, func(i, j int) bool { return dropped[i].FullName() < dropped[j].FullName() })
	return active, dropped
}

// coveredAfter reports whether a statement that can affect a table occurs
// after the table's CREATE TABLE statement. This prevents a dynamic RLS block
// in an early migration from being treated as permanent coverage for tables
// created months later.
func coveredAfter(t table, positions []sourcePosition) bool {
	for _, position := range positions {
		if position.File > t.File || (position.File == t.File && position.Offset > t.Offset) {
			return true
		}
	}
	return false
}

// indexMatches scans each migration independently and preserves statement
// order. Schema and table are case-folded to match PostgreSQL identifiers.
func indexMatches(files []string, re *regexp.Regexp) (map[string][]sourcePosition, error) {
	out := map[string][]sourcePosition{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		body := string(data)
		if re == dropTableRe {
			body = maskSQLCommentsAndStrings(body)
		}
		for _, match := range re.FindAllStringSubmatchIndex(body, -1) {
			if len(match) < 6 || match[4] < 0 {
				continue
			}
			schema := "public"
			if match[2] >= 0 {
				schema = strings.ToLower(strings.TrimSpace(body[match[2]:match[3]]))
			}
			name := strings.ToLower(strings.TrimSpace(body[match[4]:match[5]]))
			if name == "" {
				continue
			}
			key := schema + "." + name
			out[key] = append(out[key], sourcePosition{File: file, Offset: match[0]})
		}
	}
	return out, nil
}

// maskSQLCommentsAndStrings replaces comments and string literals with spaces
// while preserving byte offsets. DROP TABLE text inside documentation,
// function bodies, or dynamic SQL strings must not change the inferred final
// schema lifecycle.
func maskSQLCommentsAndStrings(body string) string {
	masked := []byte(body)
	mask := func(start, end int) {
		for i := start; i < end; i++ {
			if masked[i] != '\n' && masked[i] != '\r' {
				masked[i] = ' '
			}
		}
	}

	for i := 0; i < len(body); {
		switch {
		case i+1 < len(body) && body[i:i+2] == "--":
			end := i + 2
			for end < len(body) && body[end] != '\n' {
				end++
			}
			mask(i, end)
			i = end

		case i+1 < len(body) && body[i:i+2] == "/*":
			end := i + 2
			depth := 1
			for end < len(body) && depth > 0 {
				switch {
				case end+1 < len(body) && body[end:end+2] == "/*":
					depth++
					end += 2
				case end+1 < len(body) && body[end:end+2] == "*/":
					depth--
					end += 2
				default:
					end++
				}
			}
			mask(i, end)
			i = end

		case body[i] == '\'':
			end := i + 1
			for end < len(body) {
				if body[end] == '\\' && end+1 < len(body) {
					end += 2
					continue
				}
				if body[end] == '\'' {
					if end+1 < len(body) && body[end+1] == '\'' {
						end += 2
						continue
					}
					end++
					break
				}
				end++
			}
			mask(i, end)
			i = end

		case body[i] == '$':
			delimiterEnd := i + 1
			for delimiterEnd < len(body) &&
				((body[delimiterEnd] >= 'a' && body[delimiterEnd] <= 'z') ||
					(body[delimiterEnd] >= 'A' && body[delimiterEnd] <= 'Z') ||
					(body[delimiterEnd] >= '0' && body[delimiterEnd] <= '9') ||
					body[delimiterEnd] == '_') {
				delimiterEnd++
			}
			if delimiterEnd >= len(body) || body[delimiterEnd] != '$' {
				i++
				continue
			}
			delimiter := body[i : delimiterEnd+1]
			closingOffset := strings.Index(body[delimiterEnd+1:], delimiter)
			if closingOffset < 0 {
				i++
				continue
			}
			end := delimiterEnd + 1 + closingOffset + len(delimiter)
			mask(i, end)
			i = end

		default:
			i++
		}
	}

	return string(masked)
}

// dynamicallyCoveredSchemas returns the ordered positions of schemas covered by a
// dynamic DO $$ … $$ block that (a) filters by column_name='tenant_id'
// and (b) executes ALTER TABLE … ENABLE ROW LEVEL SECURITY. The
// finalize_jsonb_rls.sql migration is the canonical example. Coverage only
// applies to tables that already existed when the block ran. Blocks guarded by
// the known-bad to_regproc('auth.jwt()') lookup are deliberately ignored.
func dynamicallyCoveredSchemas(files []string) (map[string][]sourcePosition, error) {
	out := map[string][]sourcePosition{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		corpus := string(data)
		for _, block := range dynamicBlockRe.FindAllStringSubmatchIndex(corpus, -1) {
			if len(block) < 4 || block[2] < 0 {
				continue
			}
			body := corpus[block[2]:block[3]]
			lower := strings.ToLower(body)
			if !strings.Contains(lower, "enable row level security") ||
				!strings.Contains(lower, "tenant_id") ||
				strings.Contains(lower, "to_regproc('auth.jwt()')") {
				continue
			}
			for _, schemaMatch := range dynamicSchemaListRe.FindAllStringSubmatch(body, -1) {
				for _, raw := range strings.Split(schemaMatch[1], ",") {
					name := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "'\""))
					if name != "" {
						out[name] = append(out[name], sourcePosition{File: file, Offset: block[0]})
					}
				}
			}
		}
	}
	return out, nil
}
