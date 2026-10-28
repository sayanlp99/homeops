export type Role = 'owner' | 'admin' | 'member' | 'viewer'

export interface User {
  id: string
  householdId: string
  username: string
  displayName: string
  email: string
  isPlatformAdmin: boolean
  /** Role in the current household; empty when the person belongs to none. */
  role: Role | ''
  color: string
  notifyPrefs: Partial<Record<NotifyKind, boolean>>
  quietStart: string | null
  quietEnd: string | null
}

export type NotifyKind = 'task_assigned' | 'task_due' | 'task_overdue' | 'ceremony' | 'meals' | 'budget' | 'kudos' | 'notes'

export interface Member {
  id: string
  username: string
  displayName: string
  email: string
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
  households?: MyHousehold[]
  mailEnabled?: boolean
  aiEnabled?: boolean
  vapidPublicKey?: string
}

export interface MyHousehold {
  id: string
  name: string
  role: Role
  members: number
  unread: number
}

export interface PendingInvite {
  code: string
  email: string
  role: Role
  invitedBy: string
  createdAt: string
  expiresAt: string
  url: string
}

export interface InviteResult {
  code: string
  url: string
  expiresAt: string
  emailSent: boolean
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
  /** The planned dish a cooking task is for. */
  mealId: string | null
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
  /** Chosen food-table entry: '' = matched by name, '-' = left out of nutrition. */
  foodKey?: string
  /** The entry used for nutrition, '' if none. */
  food?: string
}

export interface Nutrients {
  kcal: number
  protein: number
  carbs: number
  fat: number
  fibre: number
  iron: number
  calcium: number
  vitC: number
  vitA: number
  folate: number
  b12: number
  zinc: number
  magnesium: number
  potassium: number
  sodium: number
}
export type NutrientKey = keyof Nutrients

export interface RecipeNutrition {
  perServing: Nutrients
  coverage: number
  missing: string[]
  contains: string[]
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
  months: number[]
  nutrition?: RecipeNutrition
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
  /** Who eats how many servings, as set by hand; null = default. */
  portions: Record<string, number> | null
  leftover: boolean
  /** Resolved portions: the explicit ones, or one serving for each member who eats this food. */
  eaters: Record<string, number>
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

// ---- nutrition ----

export type Sex = 'female' | 'male'
export type Activity = 'sedentary' | 'light' | 'moderate' | 'active'
export type Goal = 'lose' | 'maintain' | 'gain'

export interface HealthProfile {
  userId: string
  sex: Sex
  birthYear: number
  heightCm: number
  weightKg: number
  activity: Activity
  goal: Goal
  avoid: string[]
  overrides: Partial<Record<NutrientKey, number>>
  updatedAt: string
  updatedBy: string | null
}

export interface Targets extends Nutrients {
  bmr: number
  tdee: number
}

export interface MemberHealth {
  userId: string
  name: string
  color: string
  profile: HealthProfile | null
  age?: number
  computed: Targets | null
  targets: Targets | null
}

export interface DayIntake {
  date: string
  totals: Nutrients
  dishes: number
  unknown: string[]
}

export interface NutrientGap {
  nutrient: NutrientKey
  kind: 'low' | 'high'
  percent: number
  ideas: { recipeId: string; title: string; amount: number; percent: number }[]
}

export interface PersonReport {
  userId: string
  name: string
  color: string
  targets: Targets | null
  days: DayIntake[]
  average: Nutrients
  counted: number
  partial: number
  gaps: NutrientGap[]
}

export interface Food {
  key: string
  name: string
  group?: string
  per100g: Nutrients
}

export interface RecipePack {
  name: string
  title: string
  description: string
  count: number
  titles: string[]
}
