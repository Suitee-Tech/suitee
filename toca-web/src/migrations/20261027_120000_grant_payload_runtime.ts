import type { MigrateDownArgs, MigrateUpArgs } from '@payloadcms/db-postgres'
import { sql } from '@payloadcms/db-postgres'

/**
 * O schema `payload` é criado pela role de migration (postgres), mas o Next
 * conecta com `toca_app`. Sem estes grants, a vitrine sobe mas o Payload não
 * consegue ler nem escrever nas próprias tabelas.
 *
 * Mantemos a migration idempotente: em bancos onde as roles ainda não existem
 * (testes/ambientes locais) o bloco apenas não faz nada.
 */
export async function up({ db }: MigrateUpArgs): Promise<void> {
  await db.execute(sql`
    DO $$
    BEGIN
      IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_app') THEN
        GRANT USAGE ON SCHEMA payload TO toca_app;
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payload TO toca_app;
        GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA payload TO toca_app;
        ALTER DEFAULT PRIVILEGES IN SCHEMA payload
          GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO toca_app;
        ALTER DEFAULT PRIVILEGES IN SCHEMA payload
          GRANT USAGE, SELECT ON SEQUENCES TO toca_app;
      END IF;

      IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'toca_service') THEN
        GRANT USAGE ON SCHEMA payload TO toca_service;
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payload TO toca_service;
        GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA payload TO toca_service;
        ALTER DEFAULT PRIVILEGES IN SCHEMA payload
          GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO toca_service;
        ALTER DEFAULT PRIVILEGES IN SCHEMA payload
          GRANT USAGE, SELECT ON SEQUENCES TO toca_service;
      END IF;
    END
    $$;
  `)
}

export async function down(_args: MigrateDownArgs): Promise<void> {
  // Grants são idempotentes e não devem ser removidos num rollback de schema.
}
