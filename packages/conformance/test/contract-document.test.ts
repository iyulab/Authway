import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { Ajv2020 } from 'ajv/dist/2020.js'
import { describe, expect, it } from 'vitest'
import { parse } from 'yaml'

// The contract itself, before any provider is asked anything: a document
// that does not hold together makes every other scenario's check meaningless.
const spec = parse(readFileSync(createRequire(import.meta.url).resolve('@authway/contract/openapi.yaml'), 'utf8')) as {
  paths: Record<string, Record<string, { operationId?: string } | unknown>>
  components: { schemas: Record<string, unknown> }
}

const methods = ['get', 'put', 'post', 'delete', 'patch', 'head', 'options', 'trace']

describe('the contract document', () => {
  it('names every operation once', () => {
    const ids = Object.values(spec.paths).flatMap((item) =>
      methods.map((m) => (item[m] as { operationId?: string } | undefined)?.operationId).filter((id): id is string => Boolean(id)),
    )
    const repeated = ids.filter((id, i) => ids.indexOf(id) !== i)
    expect(repeated).toEqual([])
  })

  it('compiles every schema', () => {
    const ajv = new Ajv2020({ strict: false, allErrors: true, logger: false })
    ajv.addSchema(spec, 'contract')
    for (const name of Object.keys(spec.components.schemas)) {
      expect(() => ajv.getSchema(`contract#/components/schemas/${name}`), name).not.toThrow()
    }
  })
})
