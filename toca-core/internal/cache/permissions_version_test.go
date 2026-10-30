package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// P1 / A5 — o bump de versão do core precisa usar a MESMA chave que os
// consumidores leem (`permsver:{tenant}` em pkg/clients). Um teste de ida e
// volta com Redis real fecha o contrato entre quem escreve e quem lê: se alguém
// renomear a chave de um lado só, a invalidação silenciosamente para de
// funcionar e a revogação volta a atrasar até 30 s.

func redisForTest(t *testing.T) *redis.Client {
	t.Helper()
	uri := os.Getenv("REDIS_URI")
	if uri == "" {
		uri = "redis://127.0.0.1:6379/0"
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Skipf("REDIS_URI inválido: %v", err)
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis indisponível: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// consumerVersionKey espelha o que pkg/clients (CoreClient) usa para ler a
// versão. Se este literal mudar em pkg/clients, este teste tem de mudar junto —
// é justamente o acoplamento que queremos tornar visível.
func consumerVersionKey(tenantID string) string { return "permsver:" + tenantID }

func TestRedisInvalidator_BumpaAChaveQueOConsumidorLe(t *testing.T) {
	rdb := redisForTest(t)
	const tenant = "tenant-a5-roundtrip"
	ctx := context.Background()
	_ = rdb.Del(ctx, consumerVersionKey(tenant)).Err()
	t.Cleanup(func() { _ = rdb.Del(context.Background(), consumerVersionKey(tenant)).Err() })

	inv := &RedisPermissionsVersionInvalidator{rdb: rdb}

	// Sem versão gravada, o consumidor cai no caminho "só TTL".
	if got, err := rdb.Get(ctx, consumerVersionKey(tenant)).Result(); err == nil && got != "" {
		t.Fatalf("chave deveria estar ausente antes do bump (valor=%q)", got)
	}

	if err := inv.BumpPermissionsVersion(ctx, tenant); err != nil {
		t.Fatalf("bump: %v", err)
	}

	v1, err := rdb.Get(ctx, consumerVersionKey(tenant)).Result()
	if err != nil {
		t.Fatalf("consumidor não encontraria a versão: %v", err)
	}
	if v1 == "" {
		t.Fatal("versão vazia após o bump — o consumidor trataria como 'sem versão' e serviria cache velho")
	}

	if err := inv.BumpPermissionsVersion(ctx, tenant); err != nil {
		t.Fatalf("segundo bump: %v", err)
	}
	v2, err := rdb.Get(ctx, consumerVersionKey(tenant)).Result()
	if err != nil {
		t.Fatal(err)
	}
	if v2 == v1 {
		t.Fatalf("a versão não mudou entre dois salvamentos (%q) — a segunda revogação não invalidaria o cache", v1)
	}
}

func TestRedisInvalidator_TenantVazioNaoEscreve(t *testing.T) {
	rdb := redisForTest(t)
	inv := &RedisPermissionsVersionInvalidator{rdb: rdb}
	if err := inv.BumpPermissionsVersion(context.Background(), ""); err != nil {
		t.Fatalf("tenant vazio deveria ser no-op, veio %v", err)
	}
}
