import { MissingCredentialError } from './errors.js'
import type { ProfileSecretRefs } from './types.js'

export interface SecretResolver {
  resolve(ref: string): Promise<string | undefined>
}

/** Resolve profile credential refs to plaintext values. Missing refs fail loud. */
export async function resolveProfileSecrets(
  resolver: SecretResolver,
  refs: ProfileSecretRefs,
): Promise<{ password?: string; privateKey?: string; passphrase?: string }> {
  const secrets: { password?: string; privateKey?: string; passphrase?: string } = {}
  if (refs.passwordRef !== undefined) {
    const value = await resolver.resolve(refs.passwordRef)
    if (value === undefined) throw new MissingCredentialError(refs.passwordRef)
    secrets.password = value
  }
  if (refs.privateKeyRef !== undefined) {
    const value = await resolver.resolve(refs.privateKeyRef)
    if (value === undefined) throw new MissingCredentialError(refs.privateKeyRef)
    secrets.privateKey = value
  }
  if (refs.passphraseRef !== undefined) {
    const value = await resolver.resolve(refs.passphraseRef)
    if (value === undefined) throw new MissingCredentialError(refs.passphraseRef)
    secrets.passphrase = value
  }
  return secrets
}
