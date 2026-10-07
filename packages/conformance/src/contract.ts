import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { Ajv2020, type ValidateFunction } from 'ajv/dist/2020.js'
import { parse } from 'yaml'

/**
 * Checks provider answers against the shared OpenAPI description
 * (@authway/contract). Every scenario that calls a login-UI endpoint through
 * `conform` therefore also proves the provider speaks the contract — the
 * same document another implementation is held to.
 */

type Spec = {
  paths: Record<string, Record<string, { responses?: Record<string, ResponseObject> }>>
  [key: string]: unknown
}
type ResponseObject = { $ref?: string; content?: Record<string, { schema?: unknown }> }

const specPath = createRequire(import.meta.url).resolve('@authway/contract/openapi.yaml')
const spec = parse(readFileSync(specPath, 'utf8')) as Spec

const ajv = new Ajv2020({ strict: false, allErrors: true })
ajv.addSchema(spec, 'contract')
const compiled = new Map<string, ValidateFunction>()

/** Escapes one JSON-pointer segment for use in a URI fragment. */
const segment = (s: string) => encodeURIComponent(s.replace(/~/g, '~0').replace(/\//g, '~1'))

function lookup(pointer: string): ResponseObject {
  let node: unknown = spec
  for (const part of pointer.replace(/^#\//, '').split('/')) {
    node = (node as Record<string, unknown>)[part.replace(/~1/g, '/').replace(/~0/g, '~')]
  }
  return node as ResponseObject
}

/**
 * Validates one answer. `path` is the template as written in the contract,
 * e.g. `/api/v1/login-flows/{flow}/password`. An answer with a status the
 * contract does not list for that operation fails too.
 */
export async function conform(method: string, path: string, res: Response): Promise<void> {
  const op = spec.paths[path]?.[method.toLowerCase()]
  if (!op) throw new Error(`The contract has no ${method} ${path}`)
  const status = String(res.status)
  let response = op.responses?.[status]
  if (!response) {
    const body = await res.clone().text()
    throw new Error(`${method} ${path} answered ${status}, which the contract does not list: ${body.slice(0, 300)}`)
  }
  let pointer = `#/paths/${segment(path)}/${method.toLowerCase()}/responses/${status}`
  if (response.$ref) {
    pointer = response.$ref
    response = lookup(response.$ref)
  }
  if (!response.content?.['application/json']) return

  const ref = `contract${pointer}/content/${segment('application/json')}/schema`
  let validate = compiled.get(ref)
  if (!validate) {
    validate = ajv.compile({ $ref: ref })
    compiled.set(ref, validate)
  }
  const text = await res.clone().text()
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    throw new Error(`${method} ${path} ${status}: the contract expects JSON, got: ${text.slice(0, 200)}`)
  }
  if (!validate(body)) {
    throw new Error(`${method} ${path} ${status} breaks the contract: ${ajv.errorsText(validate.errors)}\n${text.slice(0, 500)}`)
  }
}
