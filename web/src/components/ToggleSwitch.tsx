interface Props {
  checked: boolean
  onChange?: (next: boolean) => void
  disabled?: boolean
  label?: string
  caption?: string
}

export function ToggleSwitch({ checked, onChange, disabled, label, caption }: Props) {
  return (
    <label className={`toggle-switch ${checked ? 'on' : 'off'}${disabled ? ' disabled' : ''}`}>
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        aria-label={label}
        onChange={(e) => onChange?.(e.target.checked)}
        onClick={(e) => e.stopPropagation()}
      />
      <span className="toggle-track" aria-hidden>
        <span className="toggle-thumb" />
      </span>
      {caption && <span className="toggle-caption">{caption}</span>}
    </label>
  )
}
