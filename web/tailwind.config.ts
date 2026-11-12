import type { Config } from 'tailwindcss'
import forms from '@tailwindcss/forms'

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['Geist', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
      },
      // Warm, near-invisible elevation: `float` for
      // popovers, tooltips and modals; never Tailwind's default shadow-md/lg/xl.
      boxShadow: {
        float: '0 4px 16px rgb(20 16 10 / 0.08)',
      },
      colors: {
        // Semantic surface tokens — resolve per theme via CSS vars
        canvas:  'rgb(var(--bg-canvas)  / <alpha-value>)',
        surface: 'rgb(var(--bg-surface) / <alpha-value>)',
        subtle:  'rgb(var(--bg-subtle)  / <alpha-value>)',
        muted:   'rgb(var(--bg-muted)   / <alpha-value>)',
        overlay: 'var(--bg-overlay)',

        // Borders
        'border-default': 'rgb(var(--border-default) / <alpha-value>)',
        'border-subtle':  'rgb(var(--border-subtle)  / <alpha-value>)',
        'border-strong':  'rgb(var(--border-strong)  / <alpha-value>)',

        // Text
        'text-primary':   'rgb(var(--text-primary)   / <alpha-value>)',
        'text-secondary': 'rgb(var(--text-secondary) / <alpha-value>)',
        'text-tertiary':  'rgb(var(--text-tertiary)  / <alpha-value>)',
        'text-muted':     'rgb(var(--text-muted)     / <alpha-value>)',

        // Ink-stamp primary button hover (fill itself reuses text-primary)
        'ink-hover': 'rgb(var(--ink-hover) / <alpha-value>)',

        // Brand accent
        accent: {
          DEFAULT:      'rgb(var(--accent)       / <alpha-value>)',
          hover:        'rgb(var(--accent-hover)  / <alpha-value>)',
          text:         'rgb(var(--accent-text)   / <alpha-value>)',
          'tint-bg':    'var(--accent-tint-bg)',
          'tint-border':'var(--accent-tint-border)',
        },

        // Semantic intent
        positive: {
          DEFAULT:   'rgb(var(--positive)        / <alpha-value>)',
          'tint-bg': 'var(--positive-tint-bg)',
        },
        negative: {
          DEFAULT:   'rgb(var(--negative)        / <alpha-value>)',
          'tint-bg': 'var(--negative-tint-bg)',
        },
        warning: {
          DEFAULT:   'rgb(var(--warning)        / <alpha-value>)',
          'tint-bg': 'var(--warning-tint-bg)',
        },
      },
    },
  },
  plugins: [forms],
}

export default config
