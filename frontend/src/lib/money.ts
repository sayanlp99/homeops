import { QueryClient, useMutation, useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { toast } from 'sonner'
import { ApiError, get, patch, post } from './api'
import type { Bill, MoneyAccount, MoneyCategory, MoneyImport, MoneyRule, MoneySummary, Transaction, TxnKind } from './types'

const inr0 = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 })
const inr2 = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2, maximumFractionDigits: 2 })

/** Paise → "₹1,23,456" (whole rupees) or "₹456.50" when there are paise. */
export function rupees(paise: number, exact = false) {
  const r = paise / 100
  return exact && paise % 100 !== 0 ? inr2.format(r) : inr0.format(Math.round(r))
}

/** Compact for chart axes: ₹1.2L, ₹45K. */
export function rupeesCompact(paise: number) {
  const r = Math.abs(paise / 100)
  const sign = paise < 0 ? '-' : ''
  if (r >= 1e7) return `${sign}₹${+(r / 1e7).toFixed(1)}Cr`
  if (r >= 1e5) return `${sign}₹${+(r / 1e5).toFixed(1)}L`
  if (r >= 1e3) return `${sign}₹${+(r / 1e3).toFixed(r >= 1e4 ? 0 : 1)}K`
  return `${sign}₹${Math.round(r)}`
}

/** "1,200.50" / "₹450" → paise; null if not a positive amount. */
export function parseRupees(s: string): number | null {
  const n = Number(s.replace(/[₹,\s]/g, ''))
  if (!isFinite(n) || n <= 0) return null
  return Math.round(n * 100)
}

export const monthKey = (d = new Date()) => format(d, 'yyyy-MM')

// ---- queries ----

export const useMoneyCategories = () => useQuery({ queryKey: ['money', 'categories'], queryFn: () => get<MoneyCategory[]>('/money/categories'), staleTime: 60_000 })
export const useAccounts = () => useQuery({ queryKey: ['money', 'accounts'], queryFn: () => get<MoneyAccount[]>('/money/accounts'), staleTime: 60_000 })
export const useSummary = (month: string) => useQuery({ queryKey: ['money', 'summary', month], queryFn: () => get<MoneySummary>(`/money/summary?month=${month}`), placeholderData: (p) => p })
export const useBills = () => useQuery({ queryKey: ['money', 'bills'], queryFn: () => get<Bill[]>('/money/bills') })
export const useRules = () => useQuery({ queryKey: ['money', 'rules'], queryFn: () => get<MoneyRule[]>('/money/rules') })
export const useImports = () => useQuery({ queryKey: ['money', 'imports'], queryFn: () => get<MoneyImport[]>('/money/imports') })

export function useTransactions(filters: { month?: string; category?: string; account?: string; paidBy?: string; q?: string }) {
  const params = new URLSearchParams(Object.entries(filters).filter(([, v]) => v) as [string, string][]).toString()
  return useQuery({ queryKey: ['money', 'transactions', params], queryFn: () => get<Transaction[]>(`/money/transactions?${params}`), placeholderData: (p) => p })
}

// ---- offline-capable quick add ----

export type NewTxn = {
  id: string
  kind: TxnKind
  amount: number
  date: string
  payee: string
  note?: string
  categoryId: string | null
  accountId?: string | null
  paidBy?: string | null
  receipt?: string
  personal?: boolean
}

export function registerMoneyMutations(qc: QueryClient) {
  qc.setMutationDefaults(['txn.create'], {
    mutationFn: (t: NewTxn) => post<Transaction>('/money/transactions', t),
    onMutate: async (t: NewTxn) => {
      await qc.cancelQueries({ queryKey: ['money', 'transactions'] })
      const draft: Transaction = {
        description: '', note: '', accountId: null, paidBy: null, receipt: '', source: 'manual', importId: null, createdBy: null,
        createdAt: new Date().toISOString(), ownerId: t.personal ? 'me' : null, ...t, categoryId: t.categoryId ?? null,
      } as Transaction
      for (const q of qc.getQueryCache().findAll({ queryKey: ['money', 'transactions'] })) {
        const params = new URLSearchParams(String(q.queryKey[2] ?? ''))
        const month = params.get('month')
        if ((!month || t.date.startsWith(month)) && !params.get('category') && !params.get('q')) {
          qc.setQueryData<Transaction[]>(q.queryKey, (old) => (old ? [draft, ...old] : old))
        }
      }
      // Bump the summary so the envelope moves instantly.
      for (const q of qc.getQueryCache().findAll({ queryKey: ['money', 'summary'] })) {
        if (t.kind !== 'expense' || !t.date.startsWith(String(q.queryKey[2]))) continue
        qc.setQueryData<MoneySummary>(q.queryKey, (s) =>
          s && {
            ...s,
            spent: s.spent + t.amount,
            categories: s.categories.map((c) => (c.categoryId === t.categoryId ? { ...c, spent: c.spent + t.amount, count: c.count + 1 } : c)),
          },
        )
      }
    },
    onError: (e: Error) => {
      if (!(e instanceof ApiError && e.status === 0)) toast.error(`Couldn't save expense: ${e.message}`)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['money'] }),
  })
}

export function useAddTxn() {
  return useMutation<Transaction, Error, NewTxn>({ mutationKey: ['txn.create'] })
}

export function useUpdateTxn() {
  return useMutation({
    mutationFn: ({ id, changes }: { id: string; changes: Record<string, unknown> }) => patch<Transaction>(`/money/transactions/${id}`, changes),
    onError: (e: Error) => toast.error(e.message),
  })
}

export const classify = (text: string) => get<{ categoryId: string | null; kind?: TxnKind }>(`/money/classify?text=${encodeURIComponent(text)}`)

export const ACCOUNT_KINDS = [
  { value: 'bank', label: 'Bank account' },
  { value: 'card', label: 'Credit card' },
  { value: 'cash', label: 'Cash' },
  { value: 'wallet', label: 'Wallet / UPI app' },
] as const

export const postJSON = post
