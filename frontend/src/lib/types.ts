export type Role = 'admin' | 'member'

export interface User {
  id: string
  householdId: string
  username: string
  displayName: string
  role: Role
  color: string
  notifyPrefs: Partial<Record<NotifyKind, boolean>>
  quietStart: string | null
  quietEnd: string | null
}

export type NotifyKind = 'task_assigned' | 'task_due' | 'task_overdue' | 'ceremony' | 'meals' | 'budget'

export interface Member {
  id: string
  username: string
  displayName: string
  role: Role
  color: string
}

export interface Household {
  id: string
  name: string
  timezone: string
  sprintStartDow: number
  planningDow: number
  planningTime: string
  autoRoll: boolean
  mealTimes: Record<'breakfast' | 'lunch' | 'snacks' | 'dinner', string>
}

export interface Label {
  id: string
  name: string
  color: string
}

export interface Bootstrap {
  setupRequired: boolean
  user: User | null
  household?: Household
  members?: Member[]
  labels?: Label[]
  vapidPublicKey?: string
}

export type TaskStatus = 'backlog' | 'todo' | 'in_progress' | 'done'

export interface Task {
  id: string
  number: number // 0 while an offline-created task hasn't synced yet
  title: string
  description: string
  status: TaskStatus
  priority: number // 0 none, 1 low, 2 medium, 3 high, 4 urgent
  points: number | null
  labels: string[]
  assigneeId: string | null
  reporterId: string | null
  sprintId: string | null
  dueDate: string | null // YYYY-MM-DD
  rank: number
  recurringId: string | null
  completedAt: string | null
  completedBy: string | null
  createdAt: string
  updatedAt: string
}

export type SprintStatus = 'planned' | 'active' | 'completed'

export interface Sprint {
  id: string
  number: number
  goal: string
  startsOn: string
  endsOn: string
  status: SprintStatus
  startedAt: string | null
  completedAt: string | null
  taskCount: number
  doneCount: number
  points: number
  donePoints: number
}

export interface Recurring {
  id: string
  title: string
  description: string
  priority: number
  points: number | null
  labels: string[]
  assigneeId: string | null
  rotate: boolean
  freq: 'daily' | 'weekly' | 'monthly'
  interval: number
  weekdays: number[]
  monthDay: number | null
  anchorDate: string
  active: boolean
}

export interface InboxItem {
  id: string
  kind: string
  title: string
  body: string
  url: string
  createdAt: string
  readAt: string | null
}

export interface PushResult {
  device: string
  ok: boolean
  status: number
  error?: string
}

// ---- meals (Phase 2) ----

export type Slot = 'breakfast' | 'lunch' | 'snacks' | 'dinner'
export type Diet = 'veg' | 'egg' | 'nonveg' | 'vegan'

export interface RecipeIngredient {
  ingredientId?: string
  name: string
  aisle?: string
  quantity: number | null
  unit: string
  note: string
}

export interface Recipe {
  id: string
  title: string
  description: string
  category: string
  mealTypes: Slot[]
  diet: Diet
  cuisine: string
  tags: string[]
  servings: number
  prepMinutes: number | null
  cookMinutes: number | null
  prepNote: string
  prepLeadHours: number
  steps: string[]
  sourceUrl: string
  photo: string
  favorite: boolean
  rating: number
  createdAt: string
  updatedAt: string
  timesCooked: number
  lastCooked: string | null
  ingredients: RecipeIngredient[]
  warnings?: string[]
}

export interface MealItem {
  id: string
  date: string
  slot: Slot
  position: number
  recipeId: string | null
  title: string
  servings: number
  note: string
  cookedAt: string | null
  cookedBy: string | null
  createdAt: string
}

export interface Suggestion {
  recipeId: string
  title: string
  photo: string
  category: string
  score: number
  reasons: string[]
}

export interface GroceryItem {
  id: string
  ingredientId: string | null
  name: string
  quantity: number | null
  unit: string
  aisle: string
  note: string
  source: 'manual' | 'plan' | 'pantry'
  checkedAt: string | null
  checkedBy: string | null
  createdBy: string | null
  createdAt: string
}

export interface PantryItem {
  id: string
  ingredientId: string
  name: string
  aisle: string
  quantity: number | null
  unit: string
  lowThreshold: number | null
  location: 'pantry' | 'fridge' | 'freezer'
  expiresOn: string | null
  updatedAt: string
}

export interface Ingredient {
  id: string
  name: string
  aisle: string
}

// ---- money (Phase 3) ----  amounts are integer paise

export interface MoneyCategory {
  id: string
  ownerId: string | null
  name: string
  icon: string
  color: string
  kind: 'expense' | 'income'
  monthlyLimit: number
  rollover: boolean
  position: number
  archived: boolean
}

export interface MoneyAccount {
  id: string
  ownerId: string | null
  name: string
  kind: 'bank' | 'card' | 'cash' | 'wallet'
  archived: boolean
  hasImportMapping: boolean
}

export type TxnKind = 'expense' | 'income' | 'transfer'

export interface Transaction {
  id: string
  ownerId: string | null
  kind: TxnKind
  amount: number
  date: string
  payee: string
  description: string
  note: string
  categoryId: string | null
  accountId: string | null
  paidBy: string | null
  receipt: string
  source: 'manual' | 'import' | 'grocery' | 'bill'
  importId: string | null
  createdBy: string | null
  createdAt: string
}

export interface MoneySummary {
  month: string
  categories: { categoryId: string; limit: number; carry: number; spent: number; count: number }[]
  budgeted: number
  spent: number
  income: number
  uncategorized: number
  byPerson: Record<string, number>
  daily: number[]
  trend: { month: string; spent: number; income: number }[]
  topPayees: { payee: string; amount: number; count: number }[] | null
}

export interface Bill {
  id: string
  ownerId: string | null
  name: string
  amount: number
  dueDay: number
  categoryId: string | null
  accountId: string | null
  remindDaysBefore: number
  lastPaidMonth: string | null
  active: boolean
}

export interface MoneyRule {
  id: string
  ownerId: string | null
  pattern: string
  categoryId: string
  kind: TxnKind
}

export interface ImportMapping {
  headerRow: number
  date: number
  description: number
  debit: number
  credit: number
  amount: number
  drCr: number
  ref: number
}

export interface ImportRow {
  line: number
  date: string
  description: string
  ref: string
  amount: number
  debit: boolean
  payee: string
  hash: string
  kind: TxnKind
  categoryId: string | null
  matched: '' | 'rule' | 'builtin'
  duplicate: boolean
  include: boolean
  alreadyRecorded?: string
}

export interface ImportPreview {
  filename: string
  mapping: ImportMapping
  header: string[] | null
  sample: string[][]
  rows: ImportRow[] | null
  warnings: string[] | null
}

export interface MoneyImport {
  id: string
  accountId: string | null
  filename: string
  rowCount: number
  imported: number
  createdBy: string | null
  createdAt: string
}
