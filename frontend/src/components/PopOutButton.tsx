// PopOutButton opens the tournament view in a bare window with no navigation
// or admin controls, for putting on a big screen while the organiser keeps this
// window to record results. The window is named, so pressing the button again
// focuses the one already open instead of stacking up duplicates.
export default function PopOutButton() {
  function popOut() {
    const w = window.open(
      '/display',
      'bgwax-display',
      'popup=yes,width=1600,height=1000',
    )
    w?.focus()
  }

  return (
    <button
      onClick={popOut}
      title="Open the tournament view in its own window for a shared screen"
      aria-label="Pop out the tournament view"
      className="shrink-0 flex items-center justify-center px-2.5 py-1.5 rounded border transition-colors"
      style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
    >
      {/* Box with an arrow leaving it — the usual "open elsewhere" mark. */}
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M14 3h7v7" />
        <path d="M21 3l-9 9" />
        <path d="M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5" />
      </svg>
    </button>
  )
}
