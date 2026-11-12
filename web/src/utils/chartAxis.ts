/**
 * Evenly spaced, round-numbered axis ticks that cover [min, max].
 *
 * The step is the smallest of 1, 2, 2.5 or 5 times a power of ten that fits
 * `target` ticks across the range, and the first and last tick are multiples of
 * it, so a $308K–$1.1M series gets $200K, $400K ... $1.2M instead of whatever
 * numbers the range happens to land on. A range that crosses zero includes 0.
 */
export function niceTicks(min: number, max: number, target = 5): number[] {
  if (!Number.isFinite(min) || !Number.isFinite(max)) return [0]
  if (min > max) [min, max] = [max, min]
  if (min === max) {
    // A flat series has no range; give it some so the axis still has steps.
    const pad = Math.abs(min) * 0.1 || 1
    min -= pad
    max += pad
  }

  const rough = (max - min) / Math.max(target - 1, 1)
  const power = 10 ** Math.floor(Math.log10(rough))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * power).find((s) => s >= rough) ?? 10 * power

  const first = Math.floor(min / step)
  const last = Math.ceil(max / step)
  const ticks: number[] = []
  for (let i = first; i <= last; i++) {
    // Multiply, then trim floating-point noise (0.1 * 3 = 0.30000000000000004).
    ticks.push(Number((i * step).toPrecision(12)))
  }
  return ticks
}

const trim = (n: number) => String(parseFloat(n.toFixed(2)))

/** "$1.2M", "$250K", "-$500K", "$0": a compact axis label that never shows a stray ".0". */
export function formatAxis(value: number): string {
  const abs = Math.abs(value)
  const sign = value < 0 ? '-' : ''
  if (abs >= 1_000_000) return `${sign}$${trim(abs / 1_000_000)}M`
  if (abs >= 1_000) return `${sign}$${trim(abs / 1_000)}K`
  return `${sign}$${trim(abs)}`
}
