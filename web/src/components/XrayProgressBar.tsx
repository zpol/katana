import { useEffect, useState } from 'react'

interface Props {
  active: boolean
  label?: string
}

/** Shown while waiting for JFrog Xray during evaluate; unmounts when active=false. */
export function XrayProgressBar({
  active,
  label = 'Querying JFrog Xray…',
}: Props) {
  const [pct, setPct] = useState(0)

  useEffect(() => {
    if (!active) {
      setPct(0)
      return
    }
    setPct(5)
    const start = Date.now()
    const expectedMs = 90_000
    const id = window.setInterval(() => {
      const t = Math.min(1, (Date.now() - start) / expectedMs)
      // Ease toward ~92% while the server is still waiting on Xray.
      const next = 5 + 87 * (1 - Math.pow(1 - t, 1.6))
      setPct(Math.min(92, next))
    }, 250)
    return () => clearInterval(id)
  }, [active])

  if (!active) return null

  return (
    <div className="xray-progress" role="status" aria-live="polite" aria-busy="true">
      <div className="xray-progress-head">
        <span className="xray-progress-label">{label}</span>
        <span className="xray-progress-pct">{Math.round(pct)}%</span>
      </div>
      <div className="xray-progress-track">
        <div className="xray-progress-fill" style={{ width: `${pct}%` }} />
        <div className="xray-progress-shimmer" aria-hidden="true" />
      </div>
      <p className="xray-progress-hint">This can take 30–90 seconds depending on the image.</p>
    </div>
  )
}
