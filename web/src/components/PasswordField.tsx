import { useState } from 'react'

type PasswordFieldProps = {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  autoComplete: 'current-password' | 'new-password'
  disabled?: boolean
  minLength?: number
  maxLength?: number
  describedBy?: string
}

export default function PasswordField({
  id,
  label,
  value,
  onChange,
  autoComplete,
  disabled = false,
  minLength,
  maxLength,
  describedBy,
}: PasswordFieldProps) {
  const [visible, setVisible] = useState(false)

  return (
    <div>
      <label htmlFor={id} className="block text-sm text-gray-700">{label}</label>
      <div className="relative mt-1">
        <input
          id={id}
          type={visible ? 'text' : 'password'}
          required
          minLength={minLength}
          maxLength={maxLength}
          autoComplete={autoComplete}
          aria-describedby={describedBy}
          disabled={disabled}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="block w-full rounded-md border border-gray-300 px-3 py-2 pr-20 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700 disabled:bg-gray-100"
        />
        <button
          type="button"
          onClick={() => setVisible((current) => !current)}
          aria-label={visible ? 'Ocultar caracteres' : 'Mostrar caracteres'}
          aria-pressed={visible}
          aria-controls={id}
          disabled={disabled}
          className="absolute inset-y-0 right-1 my-1 rounded px-2 text-xs font-medium text-blue-700 hover:bg-blue-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-700 disabled:opacity-50"
        >
          {visible ? 'Ocultar' : 'Mostrar'}
        </button>
      </div>
    </div>
  )
}
