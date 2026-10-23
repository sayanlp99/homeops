import { useMutation, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { CheckCircle2, EyeOff, Flame, Lock, Plus, ThumbsUp, Trash2, Unlock, User } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Menu, MenuItem } from '@/components/Menu'
import { Avatar, Badge, Button, Card, CardHeader, EmptyState, PageHeader, Select, Spinner } from '@/components/ui'
import { del, patch, post } from '@/lib/api'
import { sprintRange, timeAgo } from '@/lib/format'
import { KUDOS_EMOJI, RETRO_COLS, useRetro, useTeam, type RetroCard, type RetroCol, type RetroState } from '@/lib/retro'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'

function useRetroAction<V, R = unknown>(fn: (v: V) => Promise<R>, success?: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['retro'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
      if (success) toast.success(success)
    },
    onError: (e: Error) => toast.error(e.message),
  })
}

export function RetroPage() {
  const { sprints, active, isLoading } = useSprints()
  const [params, setParams] = useSearchParams()
  const options = sprints.filter((s) => s.status !== 'planned')
  const sprintId = params.get('sprint') ?? active?.id ?? options[0]?.id
  const sprint = sprints.find((s) => s.id === sprintId)
  const { data, isLoading: loading } = useRetro(sprintId)

  if (isLoading) return <Spinner className="mx-auto mt-16" />
  return (
    <div>
      <PageHeader
        title="Retro"
        subtitle={sprint ? `Sprint ${sprint.number} · ${sprintRange(sprint)}` : 'Look back on the week together'}
        actions={
          options.length > 0 && (
            <Select
              value={sprintId ?? ''}
              onChange={(e) =>
                setParams((p) => {
                  p.set('sprint', e.target.value)
                  return p
                })
              }
              className="h-8 w-auto"
              aria-label="Sprint"
            >
              {options.map((s) => (
                <option key={s.id} value={s.id}>
                  Sprint {s.number} · {sprintRange(s)}
                  {s.status === 'active' ? ' (current)' : ''}
                </option>
              ))}
            </Select>
          )
        }
      />
      {!sprintId ? (
        <Card>
          <EmptyState title="No sprint yet" description="Retros open once a sprint has started." />
        </Card>
      ) : loading || !data ? (
        <Spinner className="mx-auto mt-10" />
      ) : (
        <RetroBoard data={data} />
      )}
      <TeamPanel />
    </div>
  )
}

function RetroBoard({ data }: { data: RetroState }) {
  const { user, partner } = useSession()
  const { retro, cards, hidden } = data
  const reveal = useRetroAction(() => post(`/retros/${retro.id}/reveal`), 'Cards revealed — time to vote')
  const close = useRetroAction(
() => post<{ tasksCreated: number }>(`/retros/${retro.id}/close`))
  const mine = cards.filter((c) => c.authorId === user.id).length
  const hiddenTotal = Object.values(hidden).reduce((a, b) => a + b, 0)
  const actions = cards.filter((c) => c.col === 'action' && !c.taskId).length

  return (
    <>
      <Card className="mb-4 flex flex-wrap items-center gap-3 p-3">
        {retro.status === 'collecting' && (
          <>
            <EyeOff className="size-4 text-subtle" />
            <p className="min-w-0 flex-1 text-[13px]">
              <b>Write privately.</b> You've added {mine} {mine === 1 ? 'card' : 'cards'}
              {partner && (
                <>
                  ; {partner.displayName} has added {hidden[partner.id] ?? 0}
                </>
              )}
              . Reveal together when you're both done.
            </p>
            <Button variant="primary" icon={<Unlock className="size-4" />} loading={reveal.isPending} disabled={mine + hiddenTotal === 0} onClick={() => reveal.mutate(undefined)}>
              Reveal cards
            </Button>
          </>
        )}
        {retro.status === 'revealed' && (
          <>
            <ThumbsUp className="size-4 text-accent" />
            <p className="min-w-0 flex-1 text-[13px]">
              <b>Discuss and vote.</b> Give action items an owner — closing the retro turns {actions} {actions === 1 ? 'action' : 'actions'} into tasks for the next sprint.
            </p>
            <Button
              variant="primary"
              icon={<CheckCircle2 className="size-4" />}
              loading={close.isPending}
              onClick={() => close.mutate(undefined, { onSuccess: (r) => toast.success(r.tasksCreated ? `Retro closed · ${r.tasksCreated} tasks added to next sprint` : 'Retro closed') })}
            >
              Close retro
            </Button>
          </>
        )}
        {retro.status === 'closed' && (
          <>
            <Lock className="size-4 text-subtle" />
            <p className="text-[13px] text-muted">This retro is closed. Action items are on the next sprint.</p>
          </>
        )}
      </Card>
      <div className="grid gap-3 lg:grid-cols-3">
        {RETRO_COLS.map((col) => (
          <Column key={col.value} col={col.value} retroId={retro.id} status={retro.status} cards={cards.filter((c) => c.col === col.value)} />
        ))}
      </div>
    </>
  )
}

function Column({ col, retroId, status, cards }: { col: RetroCol; retroId: string; status: string; cards: RetroCard[] }) {
  const meta = RETRO_COLS.find((c) => c.value === col)!
  const [text, setText] = useState('')
  const add = useRetroAction((t: string) => post(`/retros/${retroId}/cards`, { col, text: t }))
  const sorted = useMemo(() => (status === 'collecting' ? cards : [...cards].sort((a, b) => b.votes - a.votes)), [cards, status])
  return (
    <section className="flex flex-col rounded-lg border border-border bg-surface-2/60">
      <header className="flex items-center gap-2 border-b border-border px-3 py-2">
        <span className="size-2 rounded-full" style={{ background: meta.tone }} />
        <span className="text-[12px] font-semibold tracking-wide uppercase">{meta.label}</span>
        <span className="text-[12px] text-subtle tabular">{cards.length}</span>
      </header>
      <div className="space-y-2 p-2">
        {sorted.map((c) => (
          <CardView key={c.id} c={c} status={status} />
        ))}
        {status !== 'closed' && (
          <form
            onSubmit={(e) => {
              e.preventDefault()
              if (text.trim()) add.mutate(text.trim(), { onSuccess: () => setText('') })
            }}
          >
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  if (text.trim()) add.mutate(text.trim(), { onSuccess: () => setText('') })
                }
              }}
              rows={2}
              placeholder={meta.hint}
              maxLength={500}
              className="w-full resize-none rounded-md border border-dashed border-border-strong bg-surface px-2.5 py-2 text-[13px] outline-none placeholder:text-subtle focus:border-accent"
            />
            {text.trim() && (
              <Button type="submit" size="sm" className="mt-1" icon={<Plus className="size-3.5" />} loading={add.isPending}>
                Add card
              </Button>
            )}
          </form>
        )}
      </div>
    </section>
  )
}

function CardView({ c, status }: { c: RetroCard; status: string }) {
  const { user, memberById, members } = useSession()
  const author = memberById.get(c.authorId)
  const assignee = c.assigneeId ? memberById.get(c.assigneeId) : undefined
  const vote = useRetroAction(() => post(`/retro-cards/${c.id}/vote`))
  const remove = useRetroAction(() => del(`/retro-cards/${c.id}`))
  const assign = useRetroAction((assigneeId: string | null) => patch(`/retro-cards/${c.id}`, { assigneeId }))
  const voted = c.voters.includes(user.id)
  const editable = status !== 'closed'
  return (
    <div className="group rounded-md border border-border bg-surface p-2.5 shadow-card">
      <p className="text-[13px] leading-snug whitespace-pre-wrap">{c.text}</p>
      <div className="mt-2 flex items-center gap-2">
        <Avatar name={author?.displayName} color={author?.color} size={18} />
        {c.col === 'action' &&
          (editable && status === 'revealed' ? (
            <Menu
              trigger={
                <button className="flex h-6 items-center gap-1 rounded border border-border px-1.5 text-[11px] text-muted hover:bg-surface-2">
                  {assignee ? <Avatar name={assignee.displayName} color={assignee.color} size={14} /> : <User className="size-3" />}
                  {assignee ? assignee.displayName : 'Owner'}
                </button>
              }
            >
              <MenuItem checked={!c.assigneeId} onSelect={() => assign.mutate(null)}>
                No owner
              </MenuItem>
              {members.map((m) => (
                <MenuItem key={m.id} checked={c.assigneeId === m.id} icon={<Avatar name={m.displayName} color={m.color} size={14} />} onSelect={() => assign.mutate(m.id)}>
                  {m.displayName}
                </MenuItem>
              ))}
            </Menu>
          ) : (
            assignee && <span className="text-[11px] text-muted">→ {assignee.displayName}</span>
          ))}
        {c.taskId && <Badge tone="success">Task created</Badge>}
        <div className="ml-auto flex items-center gap-1">
          {status !== 'collecting' && (
            <button
              disabled={status !== 'revealed'}
              onClick={() => vote.mutate(undefined)}
              className={clsx('flex h-6 items-center gap-1 rounded px-1.5 text-[11px] font-medium tabular', voted ? 'bg-accent-soft text-accent' : 'text-muted hover:bg-surface-2')}
              aria-label={voted ? 'Remove vote' : 'Vote'}
            >
              <ThumbsUp className="size-3" /> {c.votes}
            </button>
          )}
          {editable && c.authorId === user.id && (
            <button onClick={() => remove.mutate(undefined)} className="rounded p-1 text-subtle opacity-0 group-hover:opacity-100 hover:text-danger focus:opacity-100" aria-label="Delete card">
              <Trash2 className="size-3.5" />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

function TeamPanel() {
  const { data } = useTeam()
  const { user, members, memberById, partner } = useSession()
  const qc = useQueryClient()
  const [emoji, setEmoji] = useState(KUDOS_EMOJI[0]!)
  const [message, setMessage] = useState('')
  useEffect(() => setMessage(''), [partner?.id])
  const send = useMutation({
    mutationFn: () => post('/kudos', { toId: partner!.id, emoji, message }),
    onSuccess: () => {
      toast.success(`${emoji} sent to ${partner!.displayName}`)
      setMessage('')
      qc.invalidateQueries({ queryKey: ['team'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  if (!data) return null
  return (
    <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_2fr]">
      <Card>
        <CardHeader title="Streaks" subtitle="Days in a row with at least one task done" />
        <ul className="divide-y divide-border">
          {members.map((m) => {
            const st = data.streaks[m.id] ?? { current: 0, best: 0, today: false }
            return (
              <li key={m.id} className="flex items-center gap-3 px-4 py-3">
                <Avatar name={m.displayName} color={m.color} size={28} />
                <div className="min-w-0 flex-1">
                  <div className="text-[13px] font-medium">{m.displayName}</div>
                  <div className="text-[12px] text-subtle">
                    Best {st.best} {st.best === 1 ? 'day' : 'days'} · {data.kudosReceived[m.id] ?? 0} kudos received
                  </div>
                </div>
                <span className={clsx('flex items-center gap-1 text-lg font-semibold tabular', st.current ? 'text-warning' : 'text-subtle')}>
                  <Flame className="size-4" /> {st.current}
                </span>
                {!st.today && st.current > 0 && m.id === user.id && <Badge tone="warning">Do one today</Badge>}
              </li>
            )
          })}
        </ul>
      </Card>
      <Card>
        <CardHeader title="Kudos" subtitle={partner ? `Say thanks to ${partner.displayName}` : 'Invite your partner to send kudos'} />
        {partner && (
          <form
            className="flex flex-wrap items-center gap-2 border-b border-border p-3"
            onSubmit={(e) => {
              e.preventDefault()
              send.mutate()
            }}
          >
            <div className="flex gap-0.5">
              {KUDOS_EMOJI.map((e) => (
                <button key={e} type="button" onClick={() => setEmoji(e)} className={clsx('size-8 rounded-md text-lg', emoji === e ? 'bg-accent-soft ring-1 ring-accent' : 'hover:bg-surface-2')} aria-label={e}>
                  {e}
                </button>
              ))}
            </div>
            <input value={message} onChange={(e) => setMessage(e.target.value)} placeholder="For what? (optional)" maxLength={200} className="h-9 min-w-40 flex-1 rounded-md border border-border bg-surface px-2.5 text-[13px] outline-none focus:border-accent lg:h-8" />
            <Button type="submit" variant="primary" loading={send.isPending}>
              Send
            </Button>
          </form>
        )}
        {!data.kudos?.length ? (
          <p className="px-4 py-5 text-[13px] text-subtle">No kudos yet. Tap the 👏 on a task your partner finished, or send one here.</p>
        ) : (
          <ul className="divide-y divide-border">
            {data.kudos.map((k) => {
              const from = memberById.get(k.fromId)
              const to = memberById.get(k.toId)
              return (
                <li key={k.id} className="flex items-start gap-3 px-4 py-2.5">
                  <span className="text-xl leading-none">{k.emoji}</span>
                  <div className="min-w-0 flex-1 text-[13px]">
                    <span className="font-medium">{from?.displayName}</span> → <span className="font-medium">{to?.displayName}</span>
                    {(k.message || k.taskTitle) && <span className="text-muted"> · {k.message || k.taskTitle}</span>}
                    <div className="text-[11px] text-subtle">{timeAgo(k.createdAt)}</div>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </div>
  )
}
