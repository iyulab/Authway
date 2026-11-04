import createClient from 'openapi-fetch'
import type { components, paths } from '@authway/contract/schema'
import { getConfig } from '../config'
import { languageHeader } from './language'

/** The shapes the API speaks, as the contract (packages/contract/openapi.yaml) has them. */
export type Schemas = components['schemas']

/**
 * A client for the API whose paths, parameters, bodies and answers are the
 * contract's: a call that names a path the contract does not have, or leaves
 * out what it asks for, does not compile. It reads the API address and the
 * global fetch when it is made, so a configuration loaded after the module
 * (and a fetch a test replaces) are the ones used. The screen's language goes
 * with every call, as a request that sends mail is worded in it.
 */
export function contract() {
  return createClient<paths>({
    baseUrl: getConfig().apiUrl,
    fetch: (request) => globalThis.fetch(request),
    headers: languageHeader(),
  })
}
