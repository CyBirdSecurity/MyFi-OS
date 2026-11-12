import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  closestCenter,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS, getEventCoordinates } from '@dnd-kit/utilities'
import { ArrowsClockwise, DotsSixVertical, EyeSlash, Plus } from '@phosphor-icons/react'
import { Button } from '../ui/Button'
import {
  DASHBOARD_WIDGETS,
  optionValue,
  useDashboardLayoutStore,
  type DashboardWidgetId,
} from '../../store/dashboardLayoutStore'
import type { Section, Width } from './layoutModel'

type Content = Partial<Record<DashboardWidgetId, ReactNode>>

const LABELS = new Map<string, string>(DASHBOARD_WIDGETS.map((w) => [w.id, w.label]))
const label = (id: string) => LABELS.get(id) ?? id
const DEFAULT_SIZE = new Map<string, string>(DASHBOARD_WIDGETS.map((w) => [w.id, w.size]))

// ─── Viewing ─────────────────────────────────────────────────────────────────

/** A vertical stack of cards. In a two-column row the last card stretches to
 *  fill its column, so a shorter column never leaves a gap under it. */
function Stack({ ids, content, fill }: { ids: string[]; content: Content; fill?: boolean }) {
  return (
    <div className={fill ? 'flex flex-col gap-4' : 'space-y-4'}>
      {ids.map((id, i) => (
        <div key={id} className={fill && i === ids.length - 1 ? 'flex-1 [&>*]:h-full' : ''}>
          {content[id as DashboardWidgetId]}
        </div>
      ))}
    </div>
  )
}

function ViewSection({ section, content }: { section: Section; content: Content }) {
  // Cards with nothing to show (no liabilities, no partner, ...) are skipped,
  // and a column left empty by that gives its space to the other one.
  const columns = section.columns
    .map((col) => col.filter((id) => content[id as DashboardWidgetId] != null))
    .filter((col) => col.length > 0)
  if (columns.length === 0) return null
  if (columns.length === 1) return <Stack ids={columns[0]} content={content} />
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {columns.map((ids, i) => (
        <Stack key={i} ids={ids} content={content} fill />
      ))}
    </div>
  )
}

// ─── Editing ─────────────────────────────────────────────────────────────────

/** A small segmented control: the width switch and a card's own options both use it. */
function ChoiceToggle<T extends string>({
  label: ariaLabel,
  choices,
  value,
  onChange,
}: {
  label: string
  choices: { value: T; label: string }[]
  value: T
  onChange: (value: T) => void
}) {
  return (
    <div className="flex rounded-md border border-border-default text-xs" role="group" aria-label={ariaLabel}>
      {choices.map((c) => (
        <button
          key={c.value}
          type="button"
          onClick={() => onChange(c.value)}
          aria-pressed={value === c.value}
          className={`px-2 py-0.5 first:rounded-l-md last:rounded-r-md transition-colors ${
            value === c.value ? 'bg-accent-tint-bg text-accent-text' : 'text-text-tertiary hover:text-text-primary'
          }`}
        >
          {c.label}
        </button>
      ))}
    </div>
  )
}

const WIDTH_CHOICES: { value: Width; label: string }[] = [
  { value: 'full', label: 'Full' },
  { value: 'half', label: 'Half' },
]

function WidgetShell({
  id,
  content,
  width,
  handleProps,
  onWidth,
  onHide,
}: {
  id: string
  content: Content
  width?: Width
  handleProps?: Record<string, unknown>
  onWidth?: (w: Width) => void
  onHide?: () => void
}) {
  const optionValues = useDashboardLayoutStore((s) => s.options)
  const setOption = useDashboardLayoutStore((s) => s.setOption)
  const options = DASHBOARD_WIDGETS.find((w) => w.id === id)?.options ?? []
  const body = content[id as DashboardWidgetId]
  return (
    <div className="rounded-lg border border-accent/40 bg-surface/40">
      <div className="flex items-center gap-1 px-2 py-1 border-b border-border-default/60 bg-subtle/60 rounded-t-lg">
        <button
          type="button"
          className="p-1 text-text-tertiary hover:text-text-primary cursor-grab active:cursor-grabbing touch-none"
          aria-label={`Drag ${label(id)}`}
          {...handleProps}
        >
          <DotsSixVertical size={18} weight="bold" />
        </button>
        <span className="flex-1 text-sm font-medium text-text-primary truncate">{label(id)}</span>
        {onWidth &&
          options.map((o) => (
            <ChoiceToggle
              key={o.key}
              label={o.label}
              choices={o.choices}
              value={optionValue(optionValues, id as DashboardWidgetId, o.key)}
              onChange={(v) => setOption(id as DashboardWidgetId, o.key, v)}
            />
          ))}
        {width && onWidth && <ChoiceToggle label="Card width" choices={WIDTH_CHOICES} value={width} onChange={onWidth} />}
        {onHide && (
          <button
            type="button"
            onClick={onHide}
            className="p-1 text-text-tertiary hover:text-text-primary"
            aria-label={`Hide ${label(id)}`}
          >
            <EyeSlash size={16} weight="bold" />
          </button>
        )}
      </div>
      {/* Real content at its real size, so the layout can be judged by eye. */}
      <div className="pointer-events-none select-none p-1.5">
        {body != null ? (
          body
        ) : (
          <div className="rounded-lg border border-dashed border-border-strong px-4 py-6 text-center text-xs text-text-tertiary">
            Appears here when there&rsquo;s something to show.
          </div>
        )}
      </div>
    </div>
  )
}

function SortableWidget({
  id,
  content,
  width,
  onWidth,
  onHide,
}: {
  id: string
  content: Content
  width: Width
  onWidth: (w: Width) => void
  onHide: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      id={`dash-card-${id}`}
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.35 : 1 }}
    >
      <WidgetShell
        id={id}
        content={content}
        width={width}
        handleProps={{ ...attributes, ...listeners }}
        onWidth={onWidth}
        onHide={onHide}
      />
    </div>
  )
}

function EditColumn({
  sectionId,
  column,
  ids,
  width,
  content,
  onWidth,
  onHide,
}: {
  sectionId: string
  column: number
  ids: string[]
  width: Width
  content: Content
  onWidth: (id: string, w: Width) => void
  onHide: (id: string) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${sectionId}:${column}` })
  return (
    <div
      ref={setNodeRef}
      className={`space-y-2 rounded-lg transition-colors ${isOver ? 'bg-accent-tint-bg' : ''}`}
    >
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        {ids.map((id) => (
          <SortableWidget
            key={id}
            id={id}
            content={content}
            width={width}
            onWidth={(w) => onWidth(id, w)}
            onHide={() => onHide(id)}
          />
        ))}
      </SortableContext>
      {ids.length === 0 && (
        <div className="flex h-14 items-center justify-center rounded-lg border border-dashed border-border-strong text-xs text-text-tertiary">
          Drop a card here to sit beside it
        </div>
      )}
    </div>
  )
}

/** The space between rows. Dropping on it (and on the few pixels of the cards
 *  beside it) makes a new full-width row. It adds no height beyond ordinary
 *  spacing: the hit area overlaps its neighbours instead. */
function Gap({ beforeSectionId }: { beforeSectionId: string | null }) {
  const { setNodeRef, isOver } = useDroppable({ id: `gap:${beforeSectionId ?? 'end'}` })
  return (
    <div ref={setNodeRef} className="relative -my-2 h-7">
      {isOver && (
        <div className="pointer-events-none absolute inset-x-0 top-1/2 flex -translate-y-1/2 items-center">
          <div className="h-1 flex-1 rounded bg-accent" />
          <span className="mx-2 rounded-full bg-accent px-2 py-0.5 text-xs text-white">New row</span>
          <div className="h-1 flex-1 rounded bg-accent" />
        </div>
      )}
    </div>
  )
}

// Where the pointer is decides the drop: a gap strip first, then a card, then a
// column's empty space. Falls back to the nearest target if the pointer is outside all.
const collisionDetection: CollisionDetection = (args) => {
  const hits = pointerWithin(args)
  const gap = hits.find((h) => String(h.id).startsWith('gap:'))
  if (gap) return [gap]
  const card = hits.find((h) => !String(h.id).startsWith('col:'))
  if (card) return [card]
  if (hits.length > 0) return [hits[0]]
  return closestCenter(args)
}

/** Lists the cards that aren't on the dashboard so one can be added back. */
function AddCardMenu({ onAdd }: { onAdd: (id: DashboardWidgetId) => void }) {
  const hidden = useDashboardLayoutStore((s) => s.hidden)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex items-center gap-1.5 rounded-lg border border-border-default bg-surface px-3 py-1.5 text-sm text-text-primary hover:border-accent"
      >
        <Plus size={14} weight="bold" />
        Add card
        {hidden.length > 0 && (
          <span className="rounded-full bg-accent px-1.5 text-xs text-white">{hidden.length}</span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 top-full z-40 mt-1 w-72 max-w-[85vw] rounded-lg border border-border-strong bg-surface p-1 shadow-float">
          {hidden.length === 0 ? (
            <p className="px-3 py-3 text-sm text-text-tertiary">Every card is already on your dashboard.</p>
          ) : (
            <ul className="max-h-72 overflow-y-auto">
              {hidden.map((id) => (
                <li key={id}>
                  <button
                    type="button"
                    onClick={() => {
                      onAdd(id)
                      setOpen(false)
                    }}
                    className="flex w-full items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-sm text-text-primary hover:bg-subtle"
                  >
                    <span>{label(id)}</span>
                    <span className="text-xs text-text-tertiary">
                      {DEFAULT_SIZE.get(id) === 'half' ? 'Half width' : 'Full width'}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}

function EditGrid({ content, onDone }: { content: Content; onDone: () => void }) {
  const { sections, moveWidget, setWidth, hideWidget, showWidget, reset } = useDashboardLayoutStore()
  const [activeId, setActiveId] = useState<string | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 150, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleAdd = (id: DashboardWidgetId) => {
    showWidget(id)
    // The card lands at the end of the dashboard; bring it into view.
    requestAnimationFrame(() =>
      document.getElementById(`dash-card-${id}`)?.scrollIntoView({ block: 'center', behavior: 'smooth' }),
    )
  }

  const handleDragEnd = ({ active, over, activatorEvent, delta }: DragEndEvent) => {
    setActiveId(null)
    if (!over) return
    const id = String(active.id) as DashboardWidgetId
    const overId = String(over.id)

    if (overId.startsWith('gap:')) {
      const sid = overId.slice(4)
      moveWidget(id, { kind: 'gap', beforeSectionId: sid === 'end' ? null : sid })
      return
    }
    if (overId.startsWith('col:')) {
      const [, sectionId, column] = overId.split(':')
      moveWidget(id, { kind: 'column', sectionId, column: Number(column), beforeId: null })
      return
    }
    if (overId === id) return

    // Dropped on another card: go before it, or after it when the cursor is in
    // its lower half. The cursor, not the dragged card's centre, decides: held by
    // its handle, a card's centre sits well below where the user is pointing.
    const start = getEventCoordinates(activatorEvent)
    for (const section of sections) {
      for (let column = 0; column < section.columns.length; column++) {
        if (!section.columns[column].includes(overId)) continue
        const items = section.columns[column].filter((x) => x !== id)
        const overIndex = items.indexOf(overId)
        const overMiddle = over.rect.top + over.rect.height / 2
        const dragged = active.rect.current.translated
        const below = start
          ? start.y + delta.y > overMiddle
          : !!dragged && dragged.top + dragged.height / 2 > overMiddle // keyboard drag
        const beforeId = below ? (items[overIndex + 1] ?? null) : overId
        moveWidget(id, { kind: 'column', sectionId: section.id, column, beforeId })
        return
      }
    }
  }

  return (
    <div>
      {/* Stays in view while scrolling a long dashboard. */}
      <div className="sticky top-0 z-30 -mx-1 mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-accent/30 bg-accent-tint-bg px-3 py-2">
        <p className="text-sm text-text-secondary">
          Drag cards by the handle. Switch a card between Full and Half width.
        </p>
        <div className="flex items-center gap-2">
          <AddCardMenu onAdd={handleAdd} />
          <button
            onClick={reset}
            className="flex items-center gap-1.5 text-sm text-text-tertiary hover:text-text-primary"
          >
            <ArrowsClockwise size={14} weight="bold" />
            Reset
          </button>
          <Button size="sm" onClick={onDone}>
            Done
          </Button>
        </div>
      </div>

      <DndContext
        sensors={sensors}
        collisionDetection={collisionDetection}
        onDragStart={({ active }) => setActiveId(String(active.id))}
        onDragCancel={() => setActiveId(null)}
        onDragEnd={handleDragEnd}
      >
        {sections.map((section) => {
          const width: Width = section.columns.length === 2 ? 'half' : 'full'
          return (
            <div key={section.id}>
              <Gap beforeSectionId={section.id} />
              <div className={`grid gap-3 ${width === 'half' ? 'grid-cols-1 md:grid-cols-2' : 'grid-cols-1'}`}>
                {section.columns.map((ids, column) => (
                  <EditColumn
                    key={column}
                    sectionId={section.id}
                    column={column}
                    ids={ids}
                    width={width}
                    content={content}
                    onWidth={(id, w) => setWidth(id as DashboardWidgetId, w)}
                    onHide={(id) => hideWidget(id as DashboardWidgetId)}
                  />
                ))}
              </div>
            </div>
          )
        })}
        <Gap beforeSectionId={null} />

        <DragOverlay>
          {activeId ? (
            <div className="opacity-90 shadow-float">
              <WidgetShell id={activeId} content={content} />
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
    </div>
  )
}

// ─── Entry point ─────────────────────────────────────────────────────────────

export function DashboardGrid({
  content,
  editing,
  onDone,
}: {
  content: Content
  editing: boolean
  onDone: () => void
}) {
  const sections = useDashboardLayoutStore((s) => s.sections)
  if (editing) return <EditGrid content={content} onDone={onDone} />
  return (
    <div className="space-y-6">
      {sections.map((section) => (
        <ViewSection key={section.id} section={section} content={content} />
      ))}
    </div>
  )
}
