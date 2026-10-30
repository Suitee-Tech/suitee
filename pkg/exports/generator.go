package exports

import (
	"context"
	"errors"
	"sync"
)

// Generator é o contrato que cada service implementa para exportar seu
// próprio domain. Stateless — o Service que invoca passa o Job
// inteiro pro generator ter acesso a filters/tenantID/etc.
//
// Returns: (content bytes, suggested file path within bucket, rowCount).
//   - content: bytes do arquivo final.
//   - filePath: caminho relativo no bucket (sem o bucket name) — ex.:
//     "exports/<tenant>/leads-2026-05-19.csv". Generators que querem
//     caminho dinâmico (com job ID, timestamp) constroem aqui.
//   - rowCount: número de linhas/registros gerados, para tracking.
type Generator interface {
	Generate(ctx context.Context, job *Job) (content []byte, filePath string, rowCount int64, err error)
}

// Registry — registro mutável de generators. Cada service no boot
// registra os generators que ele cobre, via RegisterGenerator. O
// Worker consulta no momento do Process.
//
// Thread-safe: leituras concorrentes do worker + escritas iniciais
// no boot. RWMutex protege.
type Registry struct {
	mu         sync.RWMutex
	generators map[generatorKey]Generator
}

type generatorKey struct {
	Entity EntityType
	Format Format
}

var defaultRegistry = &Registry{generators: map[generatorKey]Generator{}}

// Register é o helper a nível de package que cada service chama no
// boot:
//
//   exports.Register(exports.EntityType("Lead"), exports.FormatCSV, &LeadCSVGenerator{...})
func Register(entity EntityType, format Format, gen Generator) {
	defaultRegistry.Register(entity, format, gen)
}

func (r *Registry) Register(entity EntityType, format Format, gen Generator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generators[generatorKey{Entity: entity, Format: format}] = gen
}

// Resolve devolve o generator pra (entity, format) ou ErrGeneratorNotFound.
// Worker faz lookup aqui antes de tentar processar.
func (r *Registry) Resolve(entity EntityType, format Format) (Generator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.generators[generatorKey{Entity: entity, Format: format}]
	if !ok {
		return nil, ErrGeneratorNotFound
	}
	return g, nil
}

// ResolveDefault — atalho do package-level. Padrão usado pelos workers
// que não montam um registry custom.
func ResolveDefault(entity EntityType, format Format) (Generator, error) {
	return defaultRegistry.Resolve(entity, format)
}

// Reset — testes podem zerar o registro entre cenários.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generators = map[generatorKey]Generator{}
}

// ResetDefault — testes podem limpar o registry global.
func ResetDefault() {
	defaultRegistry.Reset()
}

// IsRegistered — útil em handlers que querem rejeitar requests para
// (entity, format) sem generator wirado, retornando 400 em vez de
// criar um job que vai falhar depois.
func IsRegistered(entity EntityType, format Format) bool {
	_, err := ResolveDefault(entity, format)
	return err == nil && !errors.Is(err, ErrGeneratorNotFound)
}
