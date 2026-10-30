# Authentication Domain

This package defines the authentication contract for `toca-core`.

## Architectural Role

`toca-core` acts as a **Consumer** of authentication tokens. It does NOT handle:
- User registration
- Login / Password validation
- Token generation
- Password resets

These responsibilities belong to the **`toca-auth`** service.

## Claims

The `Claims` struct defined here represents the data contract for the JWTs issued by `toca-auth`.
It must be kept in sync with `toca-auth/internal/domain/auth/auth.go`.

## Middleware

The `AuthMiddleware` in `internal/api/middleware` enforces that all protected requests must carry a valid JWT signed by `toca-auth`.
It extracts the `Claims` and populates the `context` with:
- `AccountID`
- `Email`
- `Role`
- `Type`
- `TenantID`
