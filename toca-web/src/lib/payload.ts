import config from '@payload-config'
import { getPayload, type Payload } from 'payload'

type PayloadGlobal = typeof globalThis & {
  __tocaWebPayload?: Promise<Payload>
}

/**
 * Single Payload instance per server process. `getPayload` is idempotent, but
 * memoizing also prevents repeated config sanitization during RSC renders.
 */
export async function getPayloadClient(): Promise<Payload> {
  const globalStore = globalThis as PayloadGlobal

  if (!globalStore.__tocaWebPayload) {
    globalStore.__tocaWebPayload = getPayload({ config }).catch((error: unknown) => {
      globalStore.__tocaWebPayload = undefined
      throw error
    })
  }

  return globalStore.__tocaWebPayload
}
