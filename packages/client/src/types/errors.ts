/**
 * Base Authway error
 */
export class AuthwayError extends Error {
  constructor(
    message: string,
    public code: string,
    public statusCode?: number
  ) {
    super(message)
    this.name = 'AuthwayError'
    Object.setPrototypeOf(this, AuthwayError.prototype)
  }
}

/**
 * Configuration error
 */
export class ConfigurationError extends AuthwayError {
  constructor(message: string) {
    super(message, 'configuration_error')
    this.name = 'ConfigurationError'
    Object.setPrototypeOf(this, ConfigurationError.prototype)
  }
}

/**
 * Network/API error
 */
export class ApiError extends AuthwayError {
  constructor(message: string, statusCode: number, public response?: any) {
    super(message, 'api_error', statusCode)
    this.name = 'ApiError'
    Object.setPrototypeOf(this, ApiError.prototype)
  }
}

/**
 * Authentication error
 */
export class AuthenticationError extends AuthwayError {
  constructor(message: string, code: string = 'authentication_error') {
    super(message, code, 401)
    this.name = 'AuthenticationError'
    Object.setPrototypeOf(this, AuthenticationError.prototype)
  }
}

/**
 * Token error
 */
export class TokenError extends AuthwayError {
  constructor(message: string, code: string = 'token_error') {
    super(message, code)
    this.name = 'TokenError'
    Object.setPrototypeOf(this, TokenError.prototype)
  }
}

/**
 * Timeout error
 */
export class TimeoutError extends AuthwayError {
  constructor(message: string = 'Request timeout') {
    super(message, 'timeout_error', 408)
    this.name = 'TimeoutError'
    Object.setPrototypeOf(this, TimeoutError.prototype)
  }
}

/**
 * Missing refresh token error
 */
export class MissingRefreshTokenError extends TokenError {
  constructor() {
    super('No refresh token available', 'missing_refresh_token')
    this.name = 'MissingRefreshTokenError'
    Object.setPrototypeOf(this, MissingRefreshTokenError.prototype)
  }
}

/**
 * Login required error
 */
export class LoginRequiredError extends AuthenticationError {
  constructor(message: string = 'Login required') {
    super(message, 'login_required')
    this.name = 'LoginRequiredError'
    Object.setPrototypeOf(this, LoginRequiredError.prototype)
  }
}

/**
 * Popup timeout error
 */
export class PopupTimeoutError extends AuthwayError {
  constructor(
    message: string = 'Popup window timed out',
    public popup: Window | null = null
  ) {
    super(message, 'timeout', 408)
    this.name = 'PopupTimeoutError'
    Object.setPrototypeOf(this, PopupTimeoutError.prototype)
  }

  toJSON() {
    return {
      name: this.name,
      message: this.message,
      code: this.code,
      statusCode: this.statusCode
    }
  }
}

/**
 * Popup cancelled error
 */
export class PopupCancelledError extends AuthwayError {
  constructor(
    message: string = 'Popup window was closed',
    public popup: Window | null = null
  ) {
    super(message, 'cancelled')
    this.name = 'PopupCancelledError'
    Object.setPrototypeOf(this, PopupCancelledError.prototype)
  }

  toJSON() {
    return {
      name: this.name,
      message: this.message,
      code: this.code
    }
  }
}
