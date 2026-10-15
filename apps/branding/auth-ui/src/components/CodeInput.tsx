import React from 'react'

interface CodeInputProps {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  autoFocus?: boolean
}

/**
 * A short code a person reads somewhere else and types here — a device's
 * code, or the code of an emailed sign-in link. Upper case as typed, wide
 * letter spacing, offered to the one-time-code autofill.
 */
const CodeInput: React.FC<CodeInputProps> = ({ id, label, value, onChange, autoFocus }) => (
  <div>
    <label htmlFor={id} className="block text-sm font-medium text-gray-700">
      {label}
    </label>
    <input
      id={id}
      name={id}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value.toUpperCase())}
      autoCapitalize="characters"
      autoComplete="one-time-code"
      spellCheck={false}
      maxLength={16}
      autoFocus={autoFocus}
      className="mt-1 appearance-none relative block w-full px-3 py-4 border border-gray-300 placeholder-gray-400 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 text-center text-2xl tracking-widest font-mono"
      placeholder="ABCD-EFGH"
    />
  </div>
)

export default CodeInput
