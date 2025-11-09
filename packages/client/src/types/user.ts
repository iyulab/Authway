/**
 * User profile from ID token
 */
export interface User {
  /**
   * Subject identifier (user ID)
   */
  sub: string

  /**
   * Email address
   */
  email: string

  /**
   * Email verification status
   */
  email_verified: boolean

  /**
   * Full name
   */
  name?: string

  /**
   * Given name (first name)
   */
  given_name?: string

  /**
   * Family name (last name)
   */
  family_name?: string

  /**
   * Profile picture URL
   */
  picture?: string

  /**
   * Locale
   */
  locale?: string

  /**
   * Timezone
   */
  zoneinfo?: string

  /**
   * Updated timestamp
   */
  updated_at?: number

  /**
   * Additional custom claims
   */
  [key: string]: any
}

/**
 * User claims (custom and standard)
 */
export interface Claims {
  /**
   * User roles
   */
  roles?: string[]

  /**
   * User permissions
   */
  permissions?: string[]

  /**
   * Tenant ID (example - can be used for multi-tenancy)
   */
  tenant_id?: string

  /**
   * Custom claims - add any custom data here
   * Examples: workspace_id, organization_id, project_id, etc.
   */
  [key: string]: any
}

/**
 * Linked identity (connected account)
 * Similar to Auth0's Identity structure
 */
export interface Identity {
  /**
   * Identity provider (e.g., 'google', 'github', 'email')
   */
  provider: string

  /**
   * User ID from the provider
   */
  user_id: string

  /**
   * Connection name
   */
  connection?: string

  /**
   * Whether this is the primary identity
   */
  is_social: boolean

  /**
   * Profile data from provider
   */
  profile_data?: {
    email?: string
    email_verified?: boolean
    name?: string
    picture?: string
    [key: string]: any
  }
}

/**
 * Options for linking accounts
 */
export interface LinkAccountOptions {
  /**
   * Provider to link (e.g., 'google', 'github')
   */
  provider: string

  /**
   * Optional connection name
   */
  connection?: string

  /**
   * Redirect URI after linking
   */
  redirectUri?: string
}
