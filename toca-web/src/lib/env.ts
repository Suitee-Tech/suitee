
/** Server-only environment helpers. Never import this from a client component. */

export function getRequiredServerEnv(name: string): string {
  const value = process.env[name]?.trim()
  if (!value) {
    throw new Error(`${name} is required`)
  }

  return value
}

export function getPayloadPublicServerUrl(): string {
  return (process.env.PAYLOAD_PUBLIC_SERVER_URL?.trim() || 'http://localhost:3000').replace(/\/+$/, '')
}

export function getSitesInternalApiUrl(): string {
  return (process.env.SITES_INTERNAL_API_URL?.trim() || 'http://localhost:8094').replace(/\/+$/, '')
}

export function getDatabaseUri(): string {
  return getRequiredServerEnv('DATABASE_URI')
}

export function getPayloadSecret(): string {
  const secret = getRequiredServerEnv('PAYLOAD_SECRET')
  if (secret.length < 32) {
    throw new Error('PAYLOAD_SECRET must be at least 32 characters long')
  }

  return secret
}

export function getInternalApiKey(): string | null {
  const value = process.env.INTERNAL_API_KEY?.trim()
  return value ? value : null
}

export function isS3Configured(): boolean {
  return Boolean(
    process.env.S3_BUCKET?.trim() &&
      process.env.S3_REGION?.trim() &&
      process.env.S3_ACCESS_KEY_ID?.trim() &&
      process.env.S3_SECRET_ACCESS_KEY?.trim(),
  )
}

export function getMediaDirectory(): string {
  return process.env.MEDIA_DIR?.trim() || 'media'
}
