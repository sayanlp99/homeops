import clsx from 'clsx'
import { addDays, format, parseISO } from 'date-fns'
import { Check, Clock, ListChecks, Pin, Plus, StickyNote, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, Card, CardHeader, Input, Switch, Textarea } from '@/components/ui'
import { timeAgo, todayISO } from '@/lib/format'
import { NOTE_COLORS, type FridgeNote, type NoteColor, type NoteItem, useNoteActions, useNotes } from '@/lib/notes'
import { uuid } from '@/lib/uuid'
import { hasRole, useSession } from '@/lib/session'

const paper: Record<NoteColor, string> = {
  yellow: 'bg-note-yellow',
  pink: 'bg-note-pink',
  blue: 'bg-note-blue',
  green: 'bg-note-green',
  orange: 'bg-note-orange',
}

// A slight, stable tilt per note, like paper stuck on by hand.
const tilts = ['-rotate-1', 'rotate-1', 'rotate-0', '-rotate-[0.6deg]', 'rotate-[0.8deg]']
const tiltFor = (id: string) => tilts[[...id].reduce((s, c) => s + c.charCodeAt(0), 0) % tilts.length]

const isoIn = (days: number) => format(addDays(new Date(), days), 'yyyy-MM-dd')

/** "Today only", "Until tomorrow", "Until Tue 28 Oct". */
function untilLabel(expiresOn: string) {
  if (expiresOn === todayISO()) return 'Today only'
  if (expiresOn === isoIn(1)) return 'Until tomorrow'
  return `Until ${format(parseISO(expiresOn), 'EEE d MMM')}`
}

/** Sticky notes on the household's fridge: quick messages both partners see on Home. */
export function FridgeNotes() {
  const { user } = useSession()
  const { data: all = [], isLoading } = useNotes()
  const today = todayISO()
  const notes = all.filter((n) => !n.expiresOn || n.expiresOn >= today) // past midnight, before the next refetch
  const [editing, setEditing] = useState<FridgeNote | 'new' | null>(null)
  const canEdit = hasRole(user, 'member')

  return (
    <Card>
      <CardHeader
        title="On the fridge"
        subtitle={notes.length ? `${notes.length} ${notes.length === 1 ? 'note' : 'notes'}` : 'Notes for each other'}
        actions={
          canEdit && (
            <Button size="sm" icon={<Plus className="size-4" />} onClick={() => setEditing('new')}>
              Note
            </Button>
          )
        }
      />
      {isLoading ? null : notes.length === 0 ? (
        <button
          disabled={!canEdit}
          onClick={() => setEditing('new')}
          className="flex w-full items-center gap-3 px-4 py-5 text-left text-[13px] text-subtle enabled:hover:text-text"
        >
          <StickyNote className="size-5 shrink-0" />
          {canEdit ? 'Nothing on the fridge. Leave a note: a reminder, a shopping ask, or just a hello.' : 'Nothing on the fridge.'}
        </button>
      ) : (
        <div className="grid grid-cols-2 gap-3 p-3 sm:grid-cols-3 lg:grid-cols-4">
          {notes.map((n) => (
            <NoteTile key={n.id} note={n} onOpen={canEdit ? () => setEditing(n) : undefined} />
          ))}
        </div>
      )}
      <NoteEditor key={editing === 'new' ? 'new' : editing?.id} note={editing} onClose={() => setEditing(null)} />
    </Card>
  )
}

function NoteTile({ note, onOpen }: { note: FridgeNote; onOpen?: () => void }) {
  const { memberById } = useSession()
  const { tick } = useNoteActions()
  const author = note.authorId ? memberById.get(note.authorId) : undefined
  const list = note.items.length > 0
  const shown = note.items.slice(0, 6)
  const done = note.items.filter((i) => i.done).length
  const summary = list ? [note.text, ...note.items.map((i) => `${i.done ? 'done: ' : ''}${i.text}`)].filter(Boolean).join(', ') : note.text
  return (
    <div
      role={onOpen ? 'button' : undefined}
      tabIndex={onOpen ? 0 : undefined}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (onOpen && e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) (e.preventDefault(), onOpen())
      }}
      aria-label={`Note${author ? ` from ${author.displayName}` : ''}: ${summary}`}
      className={clsx(
        'relative flex min-h-28 flex-col rounded-sm p-3 text-left text-note-ink shadow-card transition-transform',
        onOpen && 'cursor-pointer outline-none hover:-translate-y-0.5 hover:rotate-0 focus-visible:ring-2 focus-visible:ring-[var(--ring)]',
        paper[note.color],
        tiltFor(note.id),
      )}
    >
      {note.pinned && <Pin className="absolute -top-1.5 right-2 size-4 rotate-45 fill-danger text-danger" aria-label="Pinned" />}
      {list ? (
        <div className="min-w-0 flex-1">
          {note.text && <p className="mb-1 line-clamp-2 text-[13px] leading-snug font-semibold break-words">{note.text}</p>}
          <ul className="space-y-0.5">
            {shown.map((it) => (
              <li key={it.id} className="flex items-start gap-1.5 text-[13px] leading-snug">
                <button
                  type="button"
                  disabled={!onOpen}
                  aria-label={`${it.done ? 'Untick' : 'Tick'} ${it.text}`}
                  aria-pressed={it.done}
                  onClick={(e) => (e.stopPropagation(), tick(note.id, it.id, !it.done))}
                  className="-m-1 shrink-0 p-1"
                >
                  <span
                    className={clsx(
                      'flex size-3.5 items-center justify-center rounded-[3px] border-[1.5px] border-current',
                      it.done ? 'bg-note-ink' : 'opacity-60',
                    )}
                  >
                    {it.done && <Check className="size-2.5" style={{ color: `var(--note-${note.color})` }} strokeWidth={4} />}
                  </span>
                </button>
                <span className={clsx('min-w-0 break-words', it.done && 'line-through opacity-50')}>{it.text}</span>
              </li>
            ))}
          </ul>
          {note.items.length > shown.length && <p className="mt-1 text-[11px] opacity-70">+{note.items.length - shown.length} more</p>}
        </div>
      ) : (
        <p className="line-clamp-6 min-w-0 flex-1 text-[13px] leading-snug break-words whitespace-pre-wrap">{note.text}</p>
      )}
      <span className="mt-2 flex items-center gap-1.5 text-[11px] opacity-70">
        {author && <Avatar name={author.displayName} color={author.color} size={16} />}
        <span className="truncate">{note.expiresOn ? untilLabel(note.expiresOn) : timeAgo(note.createdAt)}</span>
        <span className="ml-auto flex shrink-0 items-center gap-1.5">
          {list && (
            <span className="tabular">
              {done}/{note.items.length}
            </span>
          )}
          {note.expiresOn && <Clock className="size-3" aria-hidden />}
        </span>
      </span>
    </div>
  )
}

/** Plain text to checklist: each line becomes an item (a single line stays the title). */
function toChecklist(text: string): { title: string; items: NoteItem[] } {
  const lines = text
    .split('\n')
    .map((l) => l.replace(/^\s*(?:[-*•]|\d+[.)])\s*/, '').trim())
    .filter(Boolean)
  if (lines.length <= 1) return { title: text.trim(), items: [] }
  return { title: '', items: lines.map((l) => ({ id: uuid(), text: l, done: false })) }
}

function NoteEditor({ note, onClose }: { note: FridgeNote | 'new' | null; onClose: () => void }) {
  const { user } = useSession()
  const { add, update, remove } = useNoteActions()
  const existing = note && note !== 'new' ? note : undefined
  const [text, setText] = useState(existing?.text ?? '')
  const [checklist, setChecklist] = useState((existing?.items.length ?? 0) > 0)
  const [items, setItems] = useState<NoteItem[]>(existing?.items ?? [])
  const [draft, setDraft] = useState('')
  const [color, setColor] = useState<NoteColor>(existing?.color ?? 'yellow')
  const [pinned, setPinned] = useState(existing?.pinned ?? false)
  const [expiresOn, setExpiresOn] = useState<string | null>(existing?.expiresOn ?? null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const trimmed = text.trim()
  const tooLong = [...trimmed].length > 500
  const finalItems = checklist ? [...items, ...(draft.trim() ? [{ id: uuid(), text: draft, done: false }] : [])].map((i) => ({ ...i, text: i.text.trim() })).filter((i) => i.text) : []
  const canSave = (trimmed || finalItems.length > 0) && !tooLong && finalItems.length <= 30
  const paperStyle = { background: `var(--note-${color})`, color: 'var(--note-ink)' }

  const save = () => {
    if (!canSave) return
    const n = { text: trimmed, color, pinned, expiresOn, items: finalItems }
    if (existing) update(existing.id, n)
    else add(user.id, n)
    onClose()
  }

  const switchMode = (toList: boolean) => {
    if (toList === checklist) return
    if (toList) {
      const c = toChecklist(text)
      setText(c.title)
      setItems(c.items)
    } else {
      setText([text.trim(), ...finalItems.map((i) => i.text)].filter(Boolean).join('\n'))
      setItems([])
      setDraft('')
    }
    setChecklist(toList)
  }

  const addDraft = () => {
    if (!draft.trim()) return
    setItems((l) => [...l, { id: uuid(), text: draft.trim(), done: false }])
    setDraft('')
  }

  return (
    <Modal
      open={!!note}
      onOpenChange={(o) => !o && onClose()}
      title={existing ? 'Edit note' : 'New fridge note'}
      description="Everyone in the household sees it on Home."
      footer={
        confirmDelete ? (
          <>
            <span className="mr-auto self-center text-[13px]">Take this note down?</span>
            <Button variant="ghost" onClick={() => setConfirmDelete(false)}>
              Keep
            </Button>
            <Button variant="danger" onClick={() => (remove(existing!.id), onClose())}>
              Delete
            </Button>
          </>
        ) : (
          <>
            {existing && (
              <Button variant="ghost" className="mr-auto text-danger" icon={<Trash2 className="size-4" />} onClick={() => setConfirmDelete(true)}>
                Delete
              </Button>
            )}
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" disabled={!canSave} onClick={save}>
              {existing ? 'Save' : 'Stick it'}
            </Button>
          </>
        )
      }
    >
      <div className="mb-3 inline-flex rounded-md border border-border p-0.5" role="radiogroup" aria-label="Note type">
        {[
          { list: false, label: 'Note', icon: <StickyNote className="size-3.5" /> },
          { list: true, label: 'Checklist', icon: <ListChecks className="size-3.5" /> },
        ].map((m) => (
          <button
            key={m.label}
            type="button"
            role="radio"
            aria-checked={checklist === m.list}
            onClick={() => switchMode(m.list)}
            className={clsx(
              'flex h-7 items-center gap-1.5 rounded px-3 text-[12px] font-medium transition-colors',
              checklist === m.list ? 'bg-surface-3 text-text' : 'text-muted hover:text-text',
            )}
          >
            {m.icon}
            {m.label}
          </button>
        ))}
      </div>
      {checklist ? (
        <div className="rounded-md border border-border p-2 shadow-card" style={paperStyle}>
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Title (optional), e.g. Shopping"
            aria-label="Checklist title"
            maxLength={600}
            className="mb-1 w-full bg-transparent px-1 py-1 font-semibold outline-none placeholder:font-normal placeholder:opacity-50"
          />
          <ul>
            {items.map((it, i) => (
              <li key={it.id} className="flex items-center gap-2 px-1">
                <input
                  type="checkbox"
                  checked={it.done}
                  aria-label={`Done: ${it.text}`}
                  onChange={(e) => setItems((l) => l.map((x) => (x.id === it.id ? { ...x, done: e.target.checked } : x)))}
                  className="size-4 shrink-0 accent-[var(--note-ink)]"
                />
                <input
                  value={it.text}
                  aria-label={`Item ${i + 1}`}
                  maxLength={200}
                  onChange={(e) => setItems((l) => l.map((x) => (x.id === it.id ? { ...x, text: e.target.value } : x)))}
                  className={clsx('min-w-0 flex-1 bg-transparent py-1.5 outline-none', it.done && 'line-through opacity-60')}
                />
                <button type="button" aria-label={`Remove ${it.text}`} onClick={() => setItems((l) => l.filter((x) => x.id !== it.id))} className="rounded p-1 opacity-60 hover:opacity-100">
                  <X className="size-4" />
                </button>
              </li>
            ))}
          </ul>
          {items.length < 30 && (
            <div className="flex items-center gap-2 px-1">
              <Plus className="size-4 shrink-0 opacity-60" />
              <input
                autoFocus={!existing}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') (e.preventDefault(), addDraft())
                }}
                onBlur={addDraft}
                placeholder="Add an item"
                enterKeyHint="next"
                maxLength={200}
                aria-label="New item"
                className="min-w-0 flex-1 bg-transparent py-1.5 outline-none placeholder:opacity-50"
              />
            </div>
          )}
        </div>
      ) : (
        <Textarea
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
          }}
          placeholder="e.g. Gas cylinder booked, delivery Tuesday"
          className="min-h-32"
          style={paperStyle}
          maxLength={600}
        />
      )}
      {tooLong && <p className="mt-1 text-[12px] text-danger">Keep it under 500 characters.</p>}
      <div className="mt-4 flex items-center justify-between gap-3">
        <div className="flex gap-2" role="radiogroup" aria-label="Colour">
          {NOTE_COLORS.map((c) => (
            <button
              key={c.value}
              role="radio"
              aria-checked={color === c.value}
              aria-label={c.label}
              title={c.label}
              onClick={() => setColor(c.value)}
              className={clsx(
                'flex size-8 items-center justify-center rounded-full border text-note-ink',
                paper[c.value],
                color === c.value ? 'border-text ring-2 ring-[var(--ring)]' : 'border-border-strong',
              )}
            >
              {color === c.value && <Check className="size-4" strokeWidth={3} />}
            </button>
          ))}
        </div>
        <label className="flex items-center gap-2 text-[13px]">
          <Pin className="size-4 text-subtle" />
          Pin to top
          <Switch checked={pinned} onChange={setPinned} label="Pin to top" />
        </label>
      </div>
      <ExpiryPicker value={expiresOn} onChange={setExpiresOn} />
    </Modal>
  )
}

const EXPIRY_PRESETS = [
  { label: 'Never', days: null },
  { label: 'After today', days: 0 },
  { label: 'After tomorrow', days: 1 },
  { label: 'In a week', days: 6 },
] as const

/** When the note takes itself down: never, a preset, or a picked date. */
function ExpiryPicker({ value, onChange }: { value: string | null; onChange: (v: string | null) => void }) {
  const presetDays = EXPIRY_PRESETS.find((p) => (p.days === null ? value === null : value === isoIn(p.days)))
  const [custom, setCustom] = useState(!!value && !presetDays)
  const chip = (active: boolean) =>
    clsx('h-8 rounded-full border px-3 text-[12px] transition-colors', active ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:text-text')
  return (
    <div className="mt-4">
      <div className="mb-1.5 flex items-center gap-1.5 text-[12px] font-medium text-muted">
        <Clock className="size-3.5" /> Take it down
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {EXPIRY_PRESETS.map((p) => (
          <button
            key={p.label}
            type="button"
            className={chip(!custom && presetDays === p)}
            onClick={() => (setCustom(false), onChange(p.days === null ? null : isoIn(p.days)))}
          >
            {p.label}
          </button>
        ))}
        <button type="button" className={chip(custom)} onClick={() => (setCustom(true), onChange(value ?? isoIn(2)))}>
          Pick a date
        </button>
      </div>
      {custom && (
        <Input
          type="date"
          aria-label="Last day on the fridge"
          className="mt-2 w-auto"
          min={todayISO()}
          value={value ?? ''}
          onChange={(e) => onChange(e.target.value || null)}
        />
      )}
      {value && <p className="mt-1.5 text-[12px] text-subtle">Shows through {format(parseISO(value), 'EEEE d MMMM')}, then comes off by itself.</p>}
    </div>
  )
}
