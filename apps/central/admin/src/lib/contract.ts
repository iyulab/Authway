// The shapes the API speaks, read from the contract (packages/contract/openapi.yaml,
// through the types generated from it) — not written out a second time here, where a
// change to the contract would leave them behind.
import type { components, paths } from '@authway/contract/schema'

type Schemas = components['schemas']

export type Webhook = Schemas['Webhook']
export type WebhookDelivery = Schemas['WebhookDelivery']
export type AuditLog = Schemas['AuditLog']
export type AuditSeverity = Schemas['AuditSeverity']
export type Invitation = Schemas['AdminInvitation']
export type Client = Schemas['Client']
export type Tenant = Schemas['Tenant']
export type TenantSettings = Schemas['TenantSettings']
export type SupportAccess = Schemas['SupportAccess']
export type Capabilities = Schemas['Capabilities']
export type ConsoleMembership = Schemas['ConsoleMembership']
export type User = Schemas['User']
export type ConsoleMe = paths['/api/v1/console/me']['get']['responses']['200']['content']['application/json']
