type RuntimeEnv = Record<string, string | undefined>

function getRuntimeEnv(): RuntimeEnv | undefined {
  return (globalThis as any).__TOCA_ENV__ as RuntimeEnv | undefined
}

export function getEnv(key: string): string | undefined {
  const runtimeValue = getRuntimeEnv()?.[key]
  if (runtimeValue !== undefined && runtimeValue !== '') return runtimeValue
  return (import.meta.env as any)?.[key] as string | undefined
}

export function getEnvBool(key: string, fallback = false): boolean {
  const value = getEnv(key)
  if (value === undefined) return fallback
  return value === 'true' || value === '1'
}
