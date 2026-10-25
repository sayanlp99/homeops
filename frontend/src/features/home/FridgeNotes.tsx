import clsx from 'clsx'
import { Check, Pin, Plus, StickyNote, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, Card, CardHeader, Switch, Textarea } from '@/components/ui'
import { timeAgo } from '@/lib/format'
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

/** Sticky notes on the household's fridge: quick messages both partners see on Home. */
export function FridgeNotes() {
  const { user } = useSession()
  const { data: notes = [], isLoading } = useNotes()
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
        <span className="truncate">{timeAgo(note.createdAt)}</span>
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
  const [confirmDelete, setConfirmDelete] = useState(false)
  const trimmed = text.trim()
  const tooLong = [...trimmed].length > 500

  const save = () => {
    if (!trimmed || tooLong) return
    if (existing) update(existing.id, { text: trimmed, color, pinned })
    else add(user.id, { text: trimmed, color, pinned })
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
    </Modal>
  )
}
