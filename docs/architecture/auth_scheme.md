## Table `audit_log_entries`

Auth: Audit trail for user actions.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `instance_id` | `uuid` |  Nullable |
| `id` | `uuid` | Primary |
| `payload` | `json` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `ip_address` | `varchar` |  |

## Table `custom_oauth_providers`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `provider_type` | `text` |  |
| `identifier` | `text` |  Unique |
| `name` | `text` |  |
| `client_id` | `text` |  |
| `client_secret` | `text` |  |
| `acceptable_client_ids` | `_text` |  |
| `scopes` | `_text` |  |
| `pkce_enabled` | `bool` |  |
| `attribute_mapping` | `jsonb` |  |
| `authorization_params` | `jsonb` |  |
| `enabled` | `bool` |  |
| `email_optional` | `bool` |  |
| `issuer` | `text` |  Nullable |
| `discovery_url` | `text` |  Nullable |
| `skip_nonce_check` | `bool` |  |
| `cached_discovery` | `jsonb` |  Nullable |
| `discovery_cached_at` | `timestamptz` |  Nullable |
| `authorization_url` | `text` |  Nullable |
| `token_url` | `text` |  Nullable |
| `userinfo_url` | `text` |  Nullable |
| `jwks_uri` | `text` |  Nullable |
| `created_at` | `timestamptz` |  |
| `updated_at` | `timestamptz` |  |

## Table `flow_state`

Stores metadata for all OAuth/SSO login flows

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  Nullable |
| `auth_code` | `text` |  Nullable |
| `code_challenge_method` | `code_challenge_method` |  Nullable |
| `code_challenge` | `text` |  Nullable |
| `provider_type` | `text` |  |
| `provider_access_token` | `text` |  Nullable |
| `provider_refresh_token` | `text` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `authentication_method` | `text` |  |
| `auth_code_issued_at` | `timestamptz` |  Nullable |
| `invite_token` | `text` |  Nullable |
| `referrer` | `text` |  Nullable |
| `oauth_client_state_id` | `uuid` |  Nullable |
| `linking_target_id` | `uuid` |  Nullable |
| `email_optional` | `bool` |  |

## Table `identities`

Auth: Stores identities associated to a user.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `provider_id` | `text` |  |
| `user_id` | `uuid` |  |
| `identity_data` | `jsonb` |  |
| `provider` | `text` |  |
| `last_sign_in_at` | `timestamptz` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `email` | `text` |  Nullable |
| `id` | `uuid` | Primary |

## Table `instances`

Auth: Manages users across multiple sites.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `uuid` | `uuid` |  Nullable |
| `raw_base_config` | `text` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |

## Table `mfa_amr_claims`

auth: stores authenticator method reference claims for multi factor authentication

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `session_id` | `uuid` |  |
| `created_at` | `timestamptz` |  |
| `updated_at` | `timestamptz` |  |
| `authentication_method` | `text` |  |
| `id` | `uuid` | Primary |

## Table `mfa_challenges`

auth: stores metadata about challenge requests made

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `factor_id` | `uuid` |  |
| `created_at` | `timestamptz` |  |
| `verified_at` | `timestamptz` |  Nullable |
| `ip_address` | `inet` |  |
| `otp_code` | `text` |  Nullable |
| `web_authn_session_data` | `jsonb` |  Nullable |

## Table `mfa_factors`

auth: stores metadata about factors

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  |
| `friendly_name` | `text` |  Nullable |
| `factor_type` | `factor_type` |  |
| `status` | `factor_status` |  |
| `created_at` | `timestamptz` |  |
| `updated_at` | `timestamptz` |  |
| `secret` | `text` |  Nullable |
| `phone` | `text` |  Nullable |
| `last_challenged_at` | `timestamptz` |  Nullable Unique |
| `web_authn_credential` | `jsonb` |  Nullable |
| `web_authn_aaguid` | `uuid` |  Nullable |
| `last_webauthn_challenge_data` | `jsonb` |  Nullable |

## Table `oauth_authorizations`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `authorization_id` | `text` |  Unique |
| `client_id` | `uuid` |  |
| `user_id` | `uuid` |  Nullable |
| `redirect_uri` | `text` |  |
| `scope` | `text` |  |
| `state` | `text` |  Nullable |
| `resource` | `text` |  Nullable |
| `code_challenge` | `text` |  Nullable |
| `code_challenge_method` | `code_challenge_method` |  Nullable |
| `response_type` | `oauth_response_type` |  |
| `status` | `oauth_authorization_status` |  |
| `authorization_code` | `text` |  Nullable Unique |
| `created_at` | `timestamptz` |  |
| `expires_at` | `timestamptz` |  |
| `approved_at` | `timestamptz` |  Nullable |
| `nonce` | `text` |  Nullable |

## Table `oauth_client_states`

Stores OAuth states for third-party provider authentication flows where Supabase acts as the OAuth client.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `provider_type` | `text` |  |
| `code_verifier` | `text` |  Nullable |
| `created_at` | `timestamptz` |  |

## Table `oauth_clients`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `client_secret_hash` | `text` |  Nullable |
| `registration_type` | `oauth_registration_type` |  |
| `redirect_uris` | `text` |  |
| `grant_types` | `text` |  |
| `client_name` | `text` |  Nullable |
| `client_uri` | `text` |  Nullable |
| `logo_uri` | `text` |  Nullable |
| `created_at` | `timestamptz` |  |
| `updated_at` | `timestamptz` |  |
| `deleted_at` | `timestamptz` |  Nullable |
| `client_type` | `oauth_client_type` |  |
| `token_endpoint_auth_method` | `text` |  |

## Table `oauth_consents`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  |
| `client_id` | `uuid` |  |
| `scopes` | `text` |  |
| `granted_at` | `timestamptz` |  |
| `revoked_at` | `timestamptz` |  Nullable |

## Table `one_time_tokens`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  |
| `token_type` | `one_time_token_type` |  |
| `token_hash` | `text` |  |
| `relates_to` | `text` |  |
| `created_at` | `timestamp` |  |
| `updated_at` | `timestamp` |  |

## Table `refresh_tokens`

Auth: Store of tokens used to refresh JWT tokens once they expire.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `instance_id` | `uuid` |  Nullable |
| `id` | `int8` | Primary |
| `token` | `varchar` |  Nullable Unique |
| `user_id` | `varchar` |  Nullable |
| `revoked` | `bool` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `parent` | `varchar` |  Nullable |
| `session_id` | `uuid` |  Nullable |

## Table `saml_providers`

Auth: Manages SAML Identity Provider connections.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `sso_provider_id` | `uuid` |  |
| `entity_id` | `text` |  Unique |
| `metadata_xml` | `text` |  |
| `metadata_url` | `text` |  Nullable |
| `attribute_mapping` | `jsonb` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `name_id_format` | `text` |  Nullable |

## Table `saml_relay_states`

Auth: Contains SAML Relay State information for each Service Provider initiated login.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `sso_provider_id` | `uuid` |  |
| `request_id` | `text` |  |
| `for_email` | `text` |  Nullable |
| `redirect_to` | `text` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `flow_state_id` | `uuid` |  Nullable |

## Table `schema_migrations`

Auth: Manages updates to the auth system.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `version` | `varchar` | Primary |

## Table `sessions`

Auth: Stores session data associated to a user.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `factor_id` | `uuid` |  Nullable |
| `aal` | `aal_level` |  Nullable |
| `not_after` | `timestamptz` |  Nullable |
| `refreshed_at` | `timestamp` |  Nullable |
| `user_agent` | `text` |  Nullable |
| `ip` | `inet` |  Nullable |
| `tag` | `text` |  Nullable |
| `oauth_client_id` | `uuid` |  Nullable |
| `refresh_token_hmac_key` | `text` |  Nullable |
| `refresh_token_counter` | `int8` |  Nullable |
| `scopes` | `text` |  Nullable |

## Table `sso_domains`

Auth: Manages SSO email address domain mapping to an SSO Identity Provider.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `sso_provider_id` | `uuid` |  |
| `domain` | `text` |  |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |

## Table `sso_providers`

Auth: Manages SSO identity provider information; see saml_providers for SAML.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `resource_id` | `text` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `disabled` | `bool` |  Nullable |

## Table `users`

Auth: Stores user login data within a secure schema.

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `instance_id` | `uuid` |  Nullable |
| `id` | `uuid` | Primary |
| `aud` | `varchar` |  Nullable |
| `role` | `varchar` |  Nullable |
| `email` | `varchar` |  Nullable |
| `encrypted_password` | `varchar` |  Nullable |
| `email_confirmed_at` | `timestamptz` |  Nullable |
| `invited_at` | `timestamptz` |  Nullable |
| `confirmation_token` | `varchar` |  Nullable |
| `confirmation_sent_at` | `timestamptz` |  Nullable |
| `recovery_token` | `varchar` |  Nullable |
| `recovery_sent_at` | `timestamptz` |  Nullable |
| `email_change_token_new` | `varchar` |  Nullable |
| `email_change` | `varchar` |  Nullable |
| `email_change_sent_at` | `timestamptz` |  Nullable |
| `last_sign_in_at` | `timestamptz` |  Nullable |
| `raw_app_meta_data` | `jsonb` |  Nullable |
| `raw_user_meta_data` | `jsonb` |  Nullable |
| `is_super_admin` | `bool` |  Nullable |
| `created_at` | `timestamptz` |  Nullable |
| `updated_at` | `timestamptz` |  Nullable |
| `phone` | `text` |  Nullable Unique |
| `phone_confirmed_at` | `timestamptz` |  Nullable |
| `phone_change` | `text` |  Nullable |
| `phone_change_token` | `varchar` |  Nullable |
| `phone_change_sent_at` | `timestamptz` |  Nullable |
| `confirmed_at` | `timestamptz` |  Nullable |
| `email_change_token_current` | `varchar` |  Nullable |
| `email_change_confirm_status` | `int2` |  Nullable |
| `banned_until` | `timestamptz` |  Nullable |
| `reauthentication_token` | `varchar` |  Nullable |
| `reauthentication_sent_at` | `timestamptz` |  Nullable |
| `is_sso_user` | `bool` |  |
| `deleted_at` | `timestamptz` |  Nullable |
| `is_anonymous` | `bool` |  |

## Table `webauthn_challenges`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  Nullable |
| `challenge_type` | `text` |  |
| `session_data` | `jsonb` |  |
| `created_at` | `timestamptz` |  |
| `expires_at` | `timestamptz` |  |

## Table `webauthn_credentials`

### Columns

| Name | Type | Constraints |
|------|------|-------------|
| `id` | `uuid` | Primary |
| `user_id` | `uuid` |  |
| `credential_id` | `bytea` |  |
| `public_key` | `bytea` |  |
| `attestation_type` | `text` |  |
| `aaguid` | `uuid` |  Nullable |
| `sign_count` | `int8` |  |
| `transports` | `jsonb` |  |
| `backup_eligible` | `bool` |  |
| `backed_up` | `bool` |  |
| `friendly_name` | `text` |  |
| `created_at` | `timestamptz` |  |
| `updated_at` | `timestamptz` |  |
| `last_used_at` | `timestamptz` |  Nullable |

