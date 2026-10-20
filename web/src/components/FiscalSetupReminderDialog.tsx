import { useEffect, useRef } from 'react'
import type { KeyboardEvent } from 'react'

type Props = {
  onContinue: () => void
  onDismiss: () => void
}

export default function FiscalSetupReminderDialog({ onContinue, onDismiss }: Props) {
  const continueButton = useRef<HTMLButtonElement>(null)
  const dismissButton = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const previouslyFocused = document.activeElement
    continueButton.current?.focus()
    return () => {
      if (previouslyFocused instanceof HTMLElement && previouslyFocused.isConnected) {
        previouslyFocused.focus()
      }
    }
  }, [])

  function dialogKeys(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onDismiss()
      return
    }
    if (event.key !== 'Tab') return
    if (!event.shiftKey && document.activeElement === dismissButton.current) {
      event.preventDefault()
      continueButton.current?.focus()
    } else if (event.shiftKey && document.activeElement === continueButton.current) {
      event.preventDefault()
      dismissButton.current?.focus()
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/60 p-4">
      <div role="dialog" aria-modal="true"
        aria-labelledby="fiscal-setup-reminder-title"
        aria-describedby="fiscal-setup-reminder-description"
        onKeyDown={dialogKeys}
        className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl">
        <div aria-hidden="true" className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-100 text-2xl text-amber-800">!</div>
        <h2 id="fiscal-setup-reminder-title" className="text-xl font-bold tracking-tight text-slate-900">
          Sua configuração fiscal ainda não está concluída
        </h2>
        <p id="fiscal-setup-reminder-description" className="mt-3 text-sm leading-relaxed text-slate-700">
          Faltam algumas informações para preparar a emissão de notas fiscais desta loja.
          Deseja continuar a configuração agora?
        </p>
        <p className="mt-2 text-xs text-slate-500">
          Preencher os dados não significa que a SEFAZ autorizou a emissão.
        </p>
        <div className="mt-6 grid gap-2">
          <button type="button" ref={continueButton} onClick={onContinue}
            className="rounded-lg bg-slate-900 px-4 py-3 text-sm font-semibold text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900">
            Continuar configuração
          </button>
          <button type="button" ref={dismissButton} onClick={onDismiss}
            className="rounded-lg border border-slate-300 px-4 py-3 text-sm font-semibold text-slate-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900">
            Agora não
          </button>
        </div>
      </div>
    </div>
  )
}
