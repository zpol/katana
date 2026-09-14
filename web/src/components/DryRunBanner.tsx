export function DryRunBanner() {
  return (
    <div className="dry-run-banner" role="status">
      <strong>DRY RUN MODE</strong>
      <span>
        Admission webhook is auditing only — pod creates are evaluated and logged but not blocked.
      </span>
    </div>
  )
}
