package database

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
)

// AdvisoryLockResult representa o desfecho de uma tentativa de
// pg_try_advisory_lock. Quando Acquired=false significa que outra
// instância (worker em outro pod, deploy concorrente, replay manual)
// já está executando o mesmo job — chamador deve fazer no-op.
type AdvisoryLockResult struct {
	Key      string
	Hash     int64
	Acquired bool
}

// TryAdvisoryLock pega um session-level lock por chave. Usa
// pg_try_advisory_lock(bigint) que é não-bloqueante: retorna
// imediatamente true se conseguiu o lock, false caso outra sessão
// já tenha pegado. Não usa pg_advisory_lock_shared — queremos
// serialização real, não overlap.
//
// O lock fica preso à conexão Postgres usada para a chamada. O
// *Releaser retornado faz pg_advisory_unlock no Release(). Caller
// deve **sempre** chamar `defer releaser.Release(ctx)` mesmo quando
// Acquired=false (no-op nesse caso), para garantir que esquecimentos
// futuros não vazem locks.
//
// key é convertido em bigint via FNV-1a hash (estável + uniforme).
// Para debug, o hash gerado vai junto no result.
//
// Uso típico (cron tick):
//
//	res, releaser, err := db.TryAdvisoryLock(ctx, "delinquency.tick")
//	if err != nil { return err }
//	defer releaser.Release(ctx)
//	if !res.Acquired {
//	    return nil // outra instância está processando
//	}
//	// ... fazer o trabalho ...
func (db *PostgresDB) TryAdvisoryLock(ctx context.Context, key string) (AdvisoryLockResult, *Releaser, error) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	lockHash := int64(h.Sum64())

	conn, err := db.DB.Conn(ctx)
	if err != nil {
		return AdvisoryLockResult{Key: key, Hash: lockHash}, &Releaser{noop: true}, fmt.Errorf("advisory lock: acquire conn: %w", err)
	}

	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockHash).Scan(&acquired); err != nil {
		_ = conn.Close()
		return AdvisoryLockResult{Key: key, Hash: lockHash}, &Releaser{noop: true}, fmt.Errorf("advisory lock: try_advisory_lock(%s/%d): %w", key, lockHash, err)
	}

	if !acquired {
		// Não pegamos o lock — devolve a conn ao pool sem desperdiçar slot.
		_ = conn.Close()
		return AdvisoryLockResult{Key: key, Hash: lockHash, Acquired: false}, &Releaser{noop: true}, nil
	}

	// Lock adquirido — releaser segura a conn até Release().
	return AdvisoryLockResult{Key: key, Hash: lockHash, Acquired: true}, &Releaser{
		hash: lockHash,
		conn: conn,
	}, nil
}

// Releaser libera o advisory lock + devolve a conn ao pool. Idempotente:
// chamar mais de uma vez é seguro (Release subsequente vira no-op).
type Releaser struct {
	hash     int64
	conn     *sql.Conn
	noop     bool
	released bool
}

// Release executa pg_advisory_unlock e fecha a conn. Erros são
// retornados para o caller log/swallow conforme contexto — mesmo no
// erro a conn é fechada para evitar leak.
func (r *Releaser) Release(ctx context.Context) error {
	if r == nil || r.noop || r.released {
		return nil
	}
	r.released = true
	defer func() {
		if r.conn != nil {
			_ = r.conn.Close()
		}
	}()
	if r.conn == nil {
		return nil
	}
	if _, err := r.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", r.hash); err != nil {
		return fmt.Errorf("advisory unlock(%d): %w", r.hash, err)
	}
	return nil
}
