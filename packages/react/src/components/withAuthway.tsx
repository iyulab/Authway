import React, { ComponentType } from 'react'
import { useAuth } from '../hooks/useAuth'
import { AuthContextValue } from '../context/AuthwayContext'

export interface WithAuthwayProps {
  auth: AuthContextValue
}

/**
 * Higher-order component that injects auth context into class components
 * Similar to Auth0's withAuth0
 *
 * @example
 * ```tsx
 * class ProfilePage extends React.Component<WithAuthwayProps> {
 *   render() {
 *     const { auth } = this.props
 *     return <div>Welcome, {auth.user?.name}!</div>
 *   }
 * }
 *
 * export default withAuthway(ProfilePage)
 * ```
 */
export function withAuthway<P extends WithAuthwayProps = WithAuthwayProps>(
  Component: ComponentType<P>
): React.FC<Omit<P, keyof WithAuthwayProps>> {
  return function WithAuthwayWrapper(props: Omit<P, keyof WithAuthwayProps>) {
    const auth = useAuth()

    return <Component {...(props as P)} auth={auth} />
  }
}
