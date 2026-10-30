// check-function-search-path scans supabase/migrations/*.sql and enforces the
// rule behind the Supabase lint `function_search_path_mutable`: every function
// created by our migrations must have a pinned `search_path`.
//
// Why this is a build-time concern: without a pinned path, unqualified names
// inside the function are resolved with the CALLER's search_path. A role that can
// create objects in a schema that comes earlier in that path can shadow a table or
// function the body depends on — the classic search_path privilege escalation. Our
// financial guard functions decide what may be posted, changed or deleted in the
// ledger, so they are precisely the ones that must not depend on the caller.
//
// A function is considered covered when either
//
//   - the CREATE FUNCTION statement itself sets the path (`SET search_path = …`),
//     which is what new code should do, or
//   - a later (or same) migration pins it with
//     `ALTER FUNCTION schema.name(...) SET search_path = …`, which is how the
//     backfill for the existing functions is done (20261011120000).
//
// Usage:
//
//	go run ./scripts/check-function-search-path            # default paths
//	go run ./scripts/check-function-search-path -dir=path -allowlist=path
//
// Exit codes:
//
//	0 — every function has a pinned search_path
//	1 — at least one function is missing it
//	2 — usage / IO error
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

// schemas we do not own: their functions come from extensions or from the
// platform (Supabase), so pinning their search_path is not ours to decide.
var foreignSchemas = map[string]bool{
	"auth": true, "storage": true, "realtime": true, "vault": true,
	"graphql": true, "graphql_public": true, "extensions": true,
	"pgbouncer": true, "supabase_functions": true, "supabase_migrations": true,
	"net": true, "pgsodium": true, "pgsodium_masks": true, "cron": true,
	"_analytics": true, "_realtime": true, "_supabase": true,
}

// createFunctionRe matches "CREATE [OR REPLACE] FUNCTION [schema.]name(".
var createFunctionRe = regexp.MustCompile(`(?i)create\s+(?:or\s+replace\s+)?function\s+(?:([a-z_][a-z0-9_]*)\.)?([a-z_][a-z0-9_]*)\s*\(`)

// pinnedInCreateRe detects a `SET search_path` clause belonging to a CREATE
// FUNCTION statement: the search stops at the end of the statement, because
// every clause of CREATE FUNCTION (LANGUAGE, AS, SET …) lives inside it.
var searchPathClauseRe = regexp.MustCompile(`(?is)\bset\s+search_path\s*(?:=|to)\s*[^;]*`)

// alterFunctionRe matches "ALTER FUNCTION [schema.]name(...)" — the signature is
// ignored on purpose: the check is per function NAME, and a name with several
// overloads is pinned by whichever ALTER targets it.
var alterFunctionRe = regexp.MustCompile(`(?i)alter\s+function\s+(?:([a-z_][a-z0-9_]*)\.)?([a-z_][a-z0-9_]*)\s*\(`)

// dynamicAlterRe matches a backfill that loops over an array of signatures and
// runs `EXECUTE format('ALTER FUNCTION %s SET search_path = …')`. The name only
// exists at runtime there, so the names come from the signature literals below —
// the same allowance the RLS check makes for dynamic DO blocks.
var dynamicAlterRe = regexp.MustCompile(`(?is)alter\s+function\s+%s\s+set\s+search_path`)

// signatureLiteralRe matches 'schema.name(…)' literals used as targets.
var signatureLiteralRe = regexp.MustCompile(`'([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)\s*\(`)

type function struct {
	Schema string
	Name   string
	File   string
}

func (f function) FullName() string { return f.Schema + "." + f.Name }

func listSQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}

// statementAt returns the text from index i up to the terminating semicolon
// (excluding it), so a clause search never leaks into the next statement.
func statementAt(src string, i int) string {
	if end := strings.IndexByte(src[i:], ';'); end >= 0 {
		return src[i : i+end]
	}
	return src[i:]
}

func loadAllowlist(path string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "#", 2)
		alvo := strings.TrimSpace(parts[0])
		motivo := ""
		if len(parts) == 2 {
			motivo = strings.TrimSpace(parts[1])
		}
		out[alvo] = motivo
	}
	return out, nil
}

func main() {
	dir := flag.String("dir", "supabase/migrations", "directory containing .sql migrations")
	allowlistPath := flag.String("allowlist", "", "path to allowlist (default: <dir>/.no-search-path-allowlist.txt)")
	verbose := flag.Bool("v", false, "print per-function status, not just failures")
	flag.Parse()

	if *allowlistPath == "" {
		*allowlistPath = filepath.Join(*dir, ".no-search-path-allowlist.txt")
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

	criadas, ordem, pinadas, err := coletar(files)
	if err != nil {
		fail(2, "parse migrations: %v", err)
	}

	faltando := funcoesSemPath(ordem, criadas, pinadas, allow)

	if *verbose {
		for _, nome := range ordem {
			status := "pinado"
			if !pinadas[nome] {
				if _, ok := allow[nome]; ok {
					status = "allowlist"
				} else {
					status = "SEM search_path"
				}
			}
			fmt.Printf("%-70s %s\n", nome, status)
		}
	}

	if len(faltando) > 0 {
		fmt.Fprintf(os.Stderr, "check-function-search-path: %d função(ões) sem search_path fixo:\n", len(faltando))
		for _, f := range faltando {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", f.FullName(), f.File)
		}
		fmt.Fprintf(os.Stderr, "\nCorrija na própria definição (`... SET search_path = pg_catalog, <schema>`) ou\nadicione um ALTER FUNCTION ... SET search_path numa migração nova. Se a função não\nfor nossa (extensão/plataforma), inclua o schema em foreignSchemas neste check.\n")
		fail(1, "search_path não fixo")
	}

	fmt.Printf("OK: %d funções nossas conferidas, todas com search_path fixo (%d arquivos).\n", len(ordem), len(files))
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-function-search-path: "+format+"\n", args...)
	os.Exit(code)
}

// funcoesSemPath devolve as funções criadas que não ficaram cobertas por
// definição, ALTER (estático ou dinâmico) nem allowlist.
func funcoesSemPath(ordem []string, criadas map[string]function, pinadas map[string]bool, allow map[string]string) []function {
	var faltando []function
	for _, nome := range ordem {
		if pinadas[nome] {
			continue
		}
		if _, ok := allow[nome]; ok {
			continue
		}
		faltando = append(faltando, criadas[nome])
	}
	return faltando
}

// coletar varre as migrações e devolve: as funções criadas (por nome), a ordem em
// que aparecem e quais ficaram com search_path fixo (na definição, por ALTER
// estático ou por backfill dinâmico).
func coletar(files []string) (map[string]function, []string, map[string]bool, error) {
	criadas := map[string]function{}
	ordem := []string{}
	pinadas := map[string]bool{}

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read %s: %w", file, err)
		}
		src := string(raw)

		for _, m := range createFunctionRe.FindAllStringSubmatchIndex(src, -1) {
			schema := "public"
			if m[2] >= 0 {
				schema = src[m[2]:m[3]]
			}
			nome := src[m[4]:m[5]]
			if foreignSchemas[schema] {
				continue
			}
			f := function{Schema: schema, Name: nome, File: filepath.Base(file)}
			stmt := statementAt(src, m[0])
			if searchPathClauseRe.MatchString(stmt) {
				pinadas[f.FullName()] = true
			}
			if _, ok := criadas[f.FullName()]; !ok {
				ordem = append(ordem, f.FullName())
			}
			criadas[f.FullName()] = f
		}

		if dynamicAlterRe.MatchString(src) {
			for _, lit := range signatureLiteralRe.FindAllStringSubmatch(src, -1) {
				pinadas[lit[1]+"."+lit[2]] = true
			}
		}

		for _, m := range alterFunctionRe.FindAllStringSubmatchIndex(src, -1) {
			schema := "public"
			if m[2] >= 0 {
				schema = src[m[2]:m[3]]
			}
			nome := src[m[4]:m[5]]
			f := function{Schema: schema, Name: nome, File: filepath.Base(file)}
			stmt := statementAt(src, m[0])
			// Um ALTER só cobre a função quando realmente fixa o caminho.
			if searchPathClauseRe.MatchString(stmt) {
				pinadas[f.FullName()] = true
			}
		}
	}

	return criadas, ordem, pinadas, nil
}
