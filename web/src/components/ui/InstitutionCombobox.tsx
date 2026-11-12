import { useState, useRef, useEffect } from 'react'
import { INSTITUTIONS } from '../../data/institutions'

const MAX_RESULTS = 8

export function InstitutionCombobox({
  value,
  onChange,
  className,
}: {
  value: string
  onChange: (value: string) => void
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState(value)
  const containerRef = useRef<HTMLDivElement>(null)

  // Keep local query in sync when value is set externally (e.g. OFX pre-fill)
  useEffect(() => {
    setQuery(value)
  }, [value])

  const matches = query.trim()
    ? INSTITUTIONS.filter(inst =>
        inst.toLowerCase().includes(query.toLowerCase()),
      ).slice(0, MAX_RESULTS)
    : INSTITUTIONS.slice(0, MAX_RESULTS)

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const v = e.target.value
    setQuery(v)
    onChange(v)
    setOpen(true)
  }

  const handleSelect = (inst: string) => {
    setQuery(inst)
    onChange(inst)
    setOpen(false)
  }

  const handleBlur = (e: React.FocusEvent) => {
    if (!containerRef.current?.contains(e.relatedTarget as Node)) {
      setOpen(false)
    }
  }

  return (
    <div ref={containerRef} className={`relative ${className ?? ''}`} onBlur={handleBlur}>
      <input
        type="text"
        value={query}
        onChange={handleInputChange}
        onFocus={() => setOpen(true)}
        placeholder="Search institutions…"
        autoComplete="off"
        className="w-full bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent"
      />
      {open && (
        <ul className="absolute z-50 mt-1 w-full bg-surface border border-border-default rounded-lg shadow-float overflow-hidden">
          {matches.length > 0 ? (
            matches.map(inst => (
              <li key={inst}>
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => handleSelect(inst)}
                  className="w-full text-left px-3 py-2 text-sm text-text-primary hover:bg-subtle transition-colors"
                >
                  {inst}
                </button>
              </li>
            ))
          ) : (
            <li className="px-3 py-2 text-sm text-text-tertiary italic">
              No match — your entry will be saved as-is
            </li>
          )}
        </ul>
      )}
    </div>
  )
}
