import React from 'react'
import { Link } from 'react-router-dom'
import { ReadinessScore } from '../api/client'
import { useChartPalette } from '../hooks/useChartPalette'
import { InfoTip } from './ui/InfoTip'

interface Props {
  score: ReadinessScore
  /** Show full component breakdown (used on ForecastPage). Default: false (summary only). */
  expanded?: boolean
}

const GRADE_COLORS: Record<string, string> = {
  A: 'text-positive',
  B: 'text-positive',
  C: 'text-warning',
  D: 'text-warning',
  F: 'text-negative',
}

function ScoreGauge({ score, grade, trackColor, palette }: { score: number; grade: string; trackColor: string; palette: ReturnType<typeof useChartPalette> }) {
  const radius = 54
  const cx = 64
  const cy = 64
  const strokeWidth = 10
  const circumference = Math.PI * radius
  const offset = circumference * (1 - score / 100)
  const gradeArcColor: Record<string, string> = {
    A: palette.gradeA, B: palette.gradeB, C: palette.gradeC, D: palette.gradeD, F: palette.gradeF,
  }
  const color = gradeArcColor[grade] ?? palette.tickMuted

  return (
    <svg viewBox="0 0 128 80" className="w-36 h-24" aria-label={`Readiness score ${score}`}>
      <path
        d={`M ${cx - radius} ${cy} A ${radius} ${radius} 0 0 1 ${cx + radius} ${cy}`}
        fill="none"
        stroke={trackColor}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
      />
      <path
        d={`M ${cx - radius} ${cy} A ${radius} ${radius} 0 0 1 ${cx + radius} ${cy}`}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        style={{ transition: 'stroke-dashoffset 0.6s ease' }}
      />
      <text
        x={cx}
        y={cy - 6}
        textAnchor="middle"
        dominantBaseline="middle"
        fill={color}
        fontSize="22"
        fontWeight="700"
      >
        {score}
      </text>
    </svg>
  )
}

function ComponentRow({ name, score, grade, insight, palette }: {
  name: string
  score: number
  grade: string
  weight: number
  insight: string
  palette: ReturnType<typeof useChartPalette>
}) {
  const gradeColor = GRADE_COLORS[grade] ?? 'text-text-secondary'
  const gradeArcColor: Record<string, string> = {
    A: palette.gradeA, B: palette.gradeB, C: palette.gradeC, D: palette.gradeD, F: palette.gradeF,
  }
  const barColor = gradeArcColor[grade] ?? palette.tickMuted

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-sm">
        <span className="text-text-primary font-medium">{name}</span>
        <span className={`font-semibold ${gradeColor}`}>
          {grade} · {score}/100
        </span>
      </div>
      <div className="h-1.5 rounded-full overflow-hidden" style={{ backgroundColor: palette.gaugeTrack }}>
        <div
          className="h-full rounded-full transition-all duration-500"
          style={{ width: `${score}%`, backgroundColor: barColor }}
        />
      </div>
      <p className="text-xs text-text-tertiary">{insight}</p>
    </div>
  )
}

export function ReadinessScoreCard({ score, expanded = false }: Props) {
  const palette = useChartPalette()
  const gradeColor = GRADE_COLORS[score.grade] ?? 'text-text-secondary'

  return (
    <div className="bg-surface rounded-lg border border-border-default p-5">
      <div className="flex items-center justify-between mb-1">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wider">
          Retirement Readiness <InfoTip term="readiness" />
        </h2>
        {!expanded && (
          <Link
            to="/forecast"
            className="text-xs text-accent-text hover:text-accent transition-colors"
          >
            See breakdown →
          </Link>
        )}
      </div>

      <div className="flex items-center gap-5">
        <ScoreGauge score={score.overall} grade={score.grade} trackColor={palette.gaugeTrack} palette={palette} />
        <div className="flex-1 min-w-0">
          <p className="text-sm text-text-primary leading-snug">{score.headline}</p>
          <p className={`mt-1 text-xs font-semibold uppercase tracking-wide ${gradeColor}`}>
            Grade {score.grade}
          </p>
        </div>
      </div>

      {expanded && (
        <div className="mt-4 space-y-4 border-t border-border-default pt-4">
          {score.components.map((c) => (
            <ComponentRow key={c.name} {...c} palette={palette} />
          ))}
        </div>
      )}
    </div>
  )
}
