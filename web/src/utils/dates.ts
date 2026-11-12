import { format, parseISO } from 'date-fns'

export function formatDate(dateStr: string, fmt = 'MMM d, yyyy'): string {
  try {
    return format(parseISO(dateStr), fmt)
  } catch {
    return dateStr
  }
}

export function formatMonthYear(dateStr: string): string {
  return formatDate(dateStr, 'MMM yyyy')
}

// Converts an API date/datetime string (e.g. "2028-12-01T00:00:00Z") into the
// YYYY-MM-DD form expected by <input type="date"> and by our API's date-only
// fields. The API returns *time.Time fields as full RFC3339 timestamps, but
// only accepts YYYY-MM-DD back — forwarding the raw value causes a silent
// parse failure server-side. Use this at every read↔write boundary for a
// date field: form initializers and any handler that echoes an existing
// date value back in a request (e.g. toggling an unrelated field).
export function toDateInputValue(dateStr?: string | null): string {
  return dateStr ? dateStr.split('T')[0] : ''
}

export function today(): string {
  return format(new Date(), 'yyyy-MM-dd')
}

export function monthsAgo(n: number): string {
  const d = new Date()
  d.setMonth(d.getMonth() - n)
  return format(d, 'yyyy-MM-dd')
}

export function yearsAgo(n: number): string {
  const d = new Date()
  d.setFullYear(d.getFullYear() - n)
  return format(d, 'yyyy-MM-dd')
}

export function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return format(d, 'yyyy-MM-dd')
}
