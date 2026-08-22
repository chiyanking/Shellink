import { describe, expect, it } from 'vitest'
import { resolveProfileSecrets } from '../src/credentials.js'
import { MissingCredentialError } from '../src/errors.js'

describe('resolveProfileSecrets', () => {
  it('resolves present refs and omits absent ones', async () => {
    const store = new Map([['PW', 'secret'], ['KEY', '-----BEGIN KEY-----'], ['PH', 'phrase']])
    const secrets = await resolveProfileSecrets({
      resolve: async (ref) => store.get(ref),
    }, { passwordRef: 'PW', privateKeyRef: 'KEY', passphraseRef: 'PH' })
    expect(secrets).toEqual({ password: 'secret', privateKey: '-----BEGIN KEY-----', passphrase: 'phrase' })
  })

  it('throws when a ref has no value', async () => {
    await expect(resolveProfileSecrets({
      resolve: async () => undefined,
    }, { passphraseRef: 'MISSING' })).rejects.toBeInstanceOf(MissingCredentialError)
    await expect(resolveProfileSecrets({
      resolve: async () => undefined,
    }, { passwordRef: 'MISSING' })).rejects.toBeInstanceOf(MissingCredentialError)
    await expect(resolveProfileSecrets({
      resolve: async () => undefined,
    }, { privateKeyRef: 'MISSING' })).rejects.toBeInstanceOf(MissingCredentialError)
  })
})
