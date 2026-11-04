import type { paths } from '@authway/contract/schema'

type Method = 'get' | 'post' | 'put' | 'patch' | 'delete'

/** The paths of the contract that answer the method. */
type PathsWith<M extends Method> = { [P in keyof paths]: paths[P] extends Record<M, unknown> ? P : never }[keyof paths]

/** The names between braces in a path of the contract: `/clients/{id}` has `id`. */
type ParamNames<P extends string> = P extends `${string}{${infer Name}}${infer Rest}` ? Name | ParamNames<Rest> : never

/**
 * The address of a call, from a path the contract has for that method. A path it does not have — a typo, a
 * route that was renamed — does not compile, and neither does a call that leaves out a parameter the path
 * names or gives one it does not. The result is the string the HTTP client takes.
 */
export function route<M extends Method, P extends PathsWith<M> & string>(
  _method: M,
  path: P,
  ...params: [ParamNames<P>] extends [never] ? [] : [Record<ParamNames<P>, string>]
): string {
  const values = (params[0] ?? {}) as Record<string, string>
  return path.replace(/\{([^}]+)\}/g, (_, name: string) => encodeURIComponent(values[name]))
}
