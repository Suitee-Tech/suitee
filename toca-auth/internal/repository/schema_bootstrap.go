package repository

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/database"
)

// BootstrapAuthSchema ensures the toca_auth schema and its tables exist.
//
// In production the schema is created and owned by the migration role
// (postgres) via supabase/migrations; the runtime role (toca_app) has no
// CREATE privilege on the database, and PostgreSQL checks that privilege even
// for `CREATE SCHEMA IF NOT EXISTS` when the schema already exists — so the
// unconditional DDL below crashes the boot with
// "permission denied for database postgres". We only run the DDL when the
// schema is actually missing (local dev, where migrations may not have run and
// the dev role can CREATE); otherwise we skip it and rely on the migrations.
func BootstrapAuthSchema(ctx context.Context, db *database.PostgresDB) error {
	var exists bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_namespace WHERE nspname = 'toca_auth'
		)
	`).Scan(&exists); err != nil {
		return fmt.Errorf("check toca_auth schema existence: %w", err)
	}
	if exists {
		return nil
	}

	_, err := db.ExecContext(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE SCHEMA IF NOT EXISTS toca_auth;

		CREATE TABLE IF NOT EXISTS toca_auth.accounts (
			id UUID PRIMARY KEY,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		ALTER TABLE toca_auth.accounts
			ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

		CREATE TABLE IF NOT EXISTS toca_auth.credentials (
			id UUID PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			account_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		ALTER TABLE toca_auth.credentials
			ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
		CREATE INDEX IF NOT EXISTS idx_auth_credentials_account_id ON toca_auth.credentials(account_id);

		CREATE TABLE IF NOT EXISTS toca_auth.invitations (
			id UUID PRIMARY KEY,
			company_id UUID NOT NULL,
			email TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		);
		ALTER TABLE toca_auth.invitations
			ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;
		CREATE INDEX IF NOT EXISTS idx_auth_invitations_company_id ON toca_auth.invitations(company_id);
		CREATE INDEX IF NOT EXISTS idx_auth_invitations_email ON toca_auth.invitations(email);
		CREATE INDEX IF NOT EXISTS idx_auth_invitations_deleted_at ON toca_auth.invitations(deleted_at);

		CREATE TABLE IF NOT EXISTS toca_auth.groups (
			id UUID PRIMARY KEY,
			company_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_auth_groups_company_id ON toca_auth.groups(company_id);

		CREATE TABLE IF NOT EXISTS toca_auth.password_resets (
			id UUID PRIMARY KEY,
			token TEXT NOT NULL UNIQUE,
			metadata JSONB NOT NULL
		);

		CREATE TABLE IF NOT EXISTS toca_auth.user_preferences (
			account_id UUID PRIMARY KEY,
			metadata JSONB NOT NULL
		);

		CREATE TABLE IF NOT EXISTS toca_auth.tenant_settings (
			tenant_id UUID PRIMARY KEY,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		);
	`)
	return err
}
