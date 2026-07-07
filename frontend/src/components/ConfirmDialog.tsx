import { useEffect } from 'react'

interface Props {
  title: string
  message: string
  confirmLabel: string
  // Danger styles the confirm button red for destructive actions.
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

// Styled replacement for window.confirm, matching the API settings modal:
// dimmed overlay, card, Cancel + a single action button. Escape or clicking
// the overlay cancels.
export default function ConfirmDialog({ title, message, confirmLabel, danger = false, onConfirm, onCancel }: Props) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.7)' }}
      onClick={onCancel}
    >
      <div
        className="w-full max-w-md rounded-xl border p-8 space-y-5"
        style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-sm uppercase tracking-widest font-black" style={{ color: '#f0f0f0' }}>
          {title}
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: '#aaa' }}>
          {message}
        </p>
        <div className="flex gap-3 justify-end pt-2">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors"
            style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            autoFocus
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors"
            style={danger
              ? { backgroundColor: 'var(--color-wax-red)', color: '#fff' }
              : { backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
