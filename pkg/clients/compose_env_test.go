package clients

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Incidente de 2026-09-30 (portal do cliente): o `toca-crm` instancia
// `clients.NewCalendarClient()`, mas o serviço `toca-crm` do
// docker-compose.prod.yml não declarava `CALENDAR_SERVICE_URL`. O client nascia
// com base URL vazia, toda chamada falhava, e o overview do portal respondia
// 503 ("Não foi possível carregar sua busca") sempre que o lead tinha visita
// agendada — o defeito só aparecia no caminho que usa o calendário.
//
// A auditoria do mesmo dia achou mais seis combinações iguais (CRM→comm,
// auth→comm, comm→auth, financial→crm/comm, deal→crm): código que compila,
// parede de configuração. Este teste lê o compose e o código e exige que cada
// serviço declare as variáveis que os seus clients consomem.
//
// O invariante é lido dos arquivos porque nem `go vet` nem o compilador
// enxergam uma variável de ambiente ausente.

var (
	composeServiceRe = regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`)
	composeEnvRe     = regexp.MustCompile(`^\s+-\s*([A-Z0-9_]+)=`)
	composeSectionRe = regexp.MustCompile(`^[a-zA-Z]`)
	clientCtorRe     = regexp.MustCompile(`clients\.New([A-Za-z]+)Client\(\)`)
	getenvRe         = regexp.MustCompile(`os\.Getenv\("([A-Z0-9_]+)"\)`)
)

// composeEnvByService devolve, por serviço, o conjunto de variáveis declaradas.
func composeEnvByService(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", path, err)
	}

	services := map[string]map[string]bool{}
	current := ""
	inEnv := false
	for _, line := range strings.Split(string(raw), "\n") {
		if match := composeServiceRe.FindStringSubmatch(line); match != nil {
			current = match[1]
			if services[current] == nil {
				services[current] = map[string]bool{}
			}
			inEnv = false
			continue
		}
		if current == "" {
			continue
		}
		if composeSectionRe.MatchString(line) {
			continue // próxima seção de topo
		}
		if strings.HasPrefix(line, "    environment:") {
			inEnv = true
			continue
		}
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			inEnv = false // outra chave do serviço (image, depends_on, …)
		}
		if inEnv {
			if match := composeEnvRe.FindStringSubmatch(line); match != nil {
				services[current][match[1]] = true
			}
		}
	}
	if len(services) == 0 {
		t.Fatalf("nenhum serviço lido de %s", path)
	}
	return services
}

// repoRoot sobe do diretório do teste até a raiz do repositório.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.prod.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("docker-compose.prod.yml não encontrado a partir do diretório do teste")
		}
		dir = parent
	}
}

// clientsUsedBy devolve os clientes que o módulo instancia em código de produção.
func clientsUsedBy(t *testing.T, moduleDir string) []string {
	t.Helper()
	used := map[string]bool{}
	err := filepath.WalkDir(moduleDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range clientCtorRe.FindAllStringSubmatch(string(raw), -1) {
			used[match[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("varredura de %s: %v", moduleDir, err)
	}
	names := make([]string, 0, len(used))
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestEveryServiceDeclaresTheEnvOfTheClientsItWires(t *testing.T) {
	root := repoRoot(t)
	services := composeEnvByService(t, filepath.Join(root, "docker-compose.prod.yml"))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir %s: %v", root, err)
	}

	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleDir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(moduleDir, "go.mod")); err != nil {
			continue
		}
		declared, isService := services[entry.Name()]
		if !isService {
			continue // módulo sem serviço no compose (pkg, ferramentas)
		}
		for _, name := range clientsUsedBy(t, moduleDir) {
			clientPath := filepath.Join(root, "pkg", "clients", strings.ToLower(name)+"_client.go")
			raw, err := os.ReadFile(clientPath)
			if err != nil {
				continue // construtor sem arquivo próprio (client inline)
			}
			for _, match := range getenvRe.FindAllStringSubmatch(string(raw), -1) {
				env := match[1]
				if !declared[env] {
					t.Errorf(
						"serviço %s usa clients.New%sClient() e não declara %s no docker-compose.prod.yml (o client nasce com base URL/credencial vazia e toda chamada falha)",
						entry.Name(), name, env,
					)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("nenhum serviço×client verificado — o teste deixou de enxergar os módulos")
	}
}
