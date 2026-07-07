import { useState } from 'react'
import { api } from '../api'

interface Props {
  onDrawn: () => void | Promise<void>
  buttonLabel?: string
}

// Button for running the group draw. How the groups are sized is configured
// on the Settings page (persisted server-side); this just triggers the draw.
// Only useful while the draw is allowed (setup, or group stage before any
// result); callers gate on that.
export default function DrawControl({ onDrawn, buttonLabel = 'Run Draw' }: Props) {
  const [drawing, setDrawing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleDraw() {
    setDrawing(true)
    setError(null)
    try {
      await api.runDraw()
      await onDrawn()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Draw failed')
    } finally {
      setDrawing(false)
    }
  }

  return (
    <div className="space-y-3">
      <button
        onClick={handleDraw}
        disabled={drawing}
        className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
      >
        {drawing ? 'Running draw…' : buttonLabel}
      </button>
      {error && (
        <p
          className="text-sm rounded px-4 py-2 border"
          style={{ color: 'var(--color-wax-red)', backgroundColor: 'rgba(232,20,46,0.08)', borderColor: 'rgba(232,20,46,0.3)' }}
        >
          {error}
        </p>
      )}
    </div>
  )
}
