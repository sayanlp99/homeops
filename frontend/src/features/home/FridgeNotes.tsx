import clsx from 'clsx'
import { addDays, format, parseISO } from 'date-fns'
import { Check, Clock, Pin, Plus, StickyNote, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, Card, CardHeader, Input, Switch, Textarea } from '@/components/ui'
import { timeAgo, todayISO } from '@/lib/format'
import { NOTE_COLORS, type FridgeNote, type NoteColor, useNoteActions, useNotes } from '@/lib/notes'
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
  const author = note.authorId ? memberById.get(note.authorId) : undefined
  return (
    <button
      onClick={onOpen}
      disabled={!onOpen}
      aria-label={`Note${author ? ` from ${author.displayName}` : ''}: ${note.text}`}
      className={clsx(
        'relative flex min-h-28 flex-col rounded-sm p-3 text-left text-note-ink shadow-card transition-transform enabled:hover:-translate-y-0.5 enabled:hover:rotate-0',
        paper[note.color],
        tiltFor(note.id),
      )}
    >
      {note.pinned && <Pin className="absolute -top-1.5 right-2 size-4 rotate-45 fill-danger text-danger" aria-label="Pinned" />}
      <p className="line-clamp-6 min-w-0 flex-1 text-[13px] leading-snug break-words whitespace-pre-wrap">{note.text}</p>
      <span className="mt-2 flex items-center gap-1.5 text-[11px] opacity-70">
        {author && <Avatar name={author.displayName} color={author.color} size={16} />}
        <span className="truncate">{note.expiresOn ? untilLabel(note.expiresOn) : timeAgo(note.createdAt)}</span>
        {note.expiresOn && <Clock className="ml-auto size-3 shrink-0" aria-hidden />}
      </span>
    </button>
  )
}

function NoteEditor({ note, onClose }: { note: FridgeNote | 'new' | null; onClose: () => void }) {
  const { user } = useSession()
  const { add, update, remove } = useNoteActions()
  const existing = note && note !== 'new' ? note : undefined
  const [text, setText] = useState(existing?.text ?? '')
  const [color, setColor] = useState<NoteColor>(existing?.color ?? 'yellow')
  const [pinned, setPinned] = useState(existing?.pinned ?? false)
  const [expiresOn, setExpiresOn] = useState<string | null>(existing?.expiresOn ?? null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const trimmed = text.trim()
  const tooLong = [...trimmed].length > 500

  const save = () => {
    if (!trimmed || tooLong) return
    if (existing) update(existing.id, { text: trimmed, color, pinned, expiresOn })
    else add(user.id, { text: trimmed, color, pinned, expiresOn })
    onClose()
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
            <Button variant="primary" disabled={!trimmed || tooLong} onClick={save}>
              {existing ? 'Save' : 'Stick it'}
            </Button>
          </>
        )
      }
    >
      <Textarea
        autoFocus
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
        }}
        placeholder="e.g. Gas cylinder booked, delivery Tuesday"
        className="min-h-32"
        style={{ background: `var(--note-${color})`, color: 'var(--note-ink)' }}
        maxLength={600}
      />
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
