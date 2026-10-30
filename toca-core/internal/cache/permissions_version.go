// Package cache contém adaptadores de cache usados pelos serviços do core.
package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/oginoino/toca/pkg/logger"
)

// PermissionsVersionInvalidator bumpa a versão da matriz de permissões de um
// tenant. Implementações reais vivem em Redis; a interface existe para o
// serviço depender de um contrato pequeno (e para os testes injetarem um fake).
//
// MOTIVAÇÃO (achado A5 da revisão de permissões de 2026-09-19): os serviços
// cacheiam `/me/permissions` por 30 s por (tenant, conta). Sem invalidação na
// escrita da matriz, uma revogação levava até 30 s para valer. O core é o dono
// da matriz, então é ele quem incrementa a versão — todas as entradas daquele
// tenant deixam de ser usadas na próxima leitura.
type PermissionsVersionInvalidator interface {
	BumpPermissionsVersion(ctx context.Context, tenantID string) error
}

// RedisPermissionsVersionInvalidator implementa o bump via Redis.
type RedisPermissionsVersionInvalidator struct {
	rdb *redis.Client
}

// NewRedisPermissionsVersionInvalidator devolve nil (e não um erro) quando
// REDIS_URI não está configurada: o core funciona sem Redis, apenas sem
// invalidação imediata (o TTL dos consumidores continua como rede de segurança).
func NewRedisPermissionsVersionInvalidator(uri string) *RedisPermissionsVersionInvalidator {
	if uri == "" {
		logger.Warn("REDIS_URI nao configurada: revogacoes de permissao vao propagar apenas pelo TTL do cache dos consumidores (ate 30s)")
		return nil
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		logger.Error("REDIS_URI invalida: cache de permissoes ficara sem invalidacao imediata", "error", err.Error())
		return nil
	}
	return &RedisPermissionsVersionInvalidator{rdb: redis.NewClient(opts)}
}

func (i *RedisPermissionsVersionInvalidator) BumpPermissionsVersion(ctx context.Context, tenantID string) error {
	if i == nil || i.rdb == nil || tenantID == "" {
		return nil
	}
	// Best-effort com timeout curto: a escrita da matriz NÃO depende do cache.
	bumpCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := i.rdb.Incr(bumpCtx, fmt.Sprintf("permsver:%s", tenantID)).Err(); err != nil {
		return fmt.Errorf("bump permissions version: %w", err)
	}
	return nil
}

// FromEnv monta o invalidador a partir de REDIS_URI (vazio = desabilitado).
func FromEnv() PermissionsVersionInvalidator {
	inv := NewRedisPermissionsVersionInvalidator(os.Getenv("REDIS_URI"))
	if inv == nil {
		return nil
	}
	return inv
}
