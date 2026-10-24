import { Home, LogOut, WifiOff } from 'lucide-react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { AppShell } from '@/components/AppShell'
import { Button, Card, Spinner } from '@/components/ui'
import { AdminPage } from '@/features/admin/AdminPage'
import { AuthLayout, ForgotPage, JoinPage, JoinSignedInPage, LoginPage, ResetPage, SetupPage } from '@/features/auth/AuthPages'
import { GroceryPage } from '@/features/grocery/GroceryPage'
import { HomePage } from '@/features/home/HomePage'
import { MealPlanPage } from '@/features/meals/MealPlanPage'
import { BillsPage } from '@/features/money/BillsPage'
import { BudgetPage } from '@/features/money/BudgetPage'
import { ImportPage } from '@/features/money/ImportPage'
import { MoneySetupPage } from '@/features/money/MoneySetupPage'
import { TransactionsPage } from '@/features/money/TransactionsPage'
import { NutritionPage } from '@/features/nutrition/NutritionPage'
import { PantryPage } from '@/features/pantry/PantryPage'
import { RecipeDetailPage } from '@/features/recipes/RecipeDetailPage'
import { RecipeEditorPage } from '@/features/recipes/RecipeEditorPage'
import { RecipesPage } from '@/features/recipes/RecipesPage'
import { RetroPage } from '@/features/retro/RetroPage'
import { RecurringPage } from '@/features/recurring/RecurringPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { SprintsPage } from '@/features/sprints/SprintsPage'
import { BacklogPage } from '@/features/tasks/BacklogPage'
import { BoardPage } from '@/features/tasks/BoardPage'
import { post } from '@/lib/api'
import { switchHousehold } from '@/lib/households'
import { clearLocalData } from '@/lib/queryClient'
import { useBootstrap } from '@/lib/session'
import type { Bootstrap } from '@/lib/types'

export function App() {
  const { data, isPending, error, refetch } = useBootstrap()

  if (isPending)
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <Spinner className="size-5" />
      </div>
    )
  if (!data)
    return (
      <div className="flex min-h-dvh flex-col items-center justify-center gap-3 px-6 text-center">
        <WifiOff className="size-6 text-subtle" />
        <p className="font-medium">Can't reach HomeOps</p>
        <p className="text-[13px] text-muted">{error?.message ?? 'Check your connection.'}</p>
        <Button onClick={() => refetch()}>Try again</Button>
      </div>
    )

  if (!data.user)
    return (
      <Routes>
        <Route path="/join/:code" element={<JoinPage />} />
        <Route path="/forgot" element={<ForgotPage />} />
        <Route path="/reset/:token" element={<ResetPage />} />
        <Route path="*" element={data.setupRequired ? <SetupPage /> : <LoginPage />} />
      </Routes>
    )

  if (!data.household)
    return (
      <Routes>
        <Route path="/join/:code" element={<JoinSignedInPage email={data.user.email} />} />
        {data.user.isPlatformAdmin && (
          <Route
            path="/admin"
            element={
              <div className="mx-auto max-w-5xl px-4 py-6">
                <AdminPage user={data.user} />
              </div>
            }
          />
        )}
        <Route path="*" element={<NoHousehold data={data} />} />
      </Routes>
    )

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<HomePage />} />
        <Route path="board" element={<BoardPage />} />
        <Route path="backlog" element={<BacklogPage />} />
        <Route path="sprints" element={<SprintsPage />} />
        <Route path="recurring" element={<RecurringPage />} />
        <Route path="tasks/:number" element={<TaskRedirect />} />
        <Route path="meals" element={<MealPlanPage />} />
        <Route path="recipes" element={<RecipesPage />} />
        <Route path="recipes/new" element={<RecipeEditorPage />} />
        <Route path="recipes/:id" element={<RecipeDetailPage />} />
        <Route path="recipes/:id/edit" element={<RecipeEditorPage key="edit" />} />
        <Route path="grocery" element={<GroceryPage />} />
        <Route path="pantry" element={<PantryPage />} />
        <Route path="nutrition" element={<NutritionPage />} />
        <Route path="budget" element={<BudgetPage />} />
        <Route path="budget/transactions" element={<TransactionsPage />} />
        <Route path="budget/bills" element={<BillsPage />} />
        <Route path="budget/import" element={<ImportPage />} />
        <Route path="budget/setup" element={<MoneySetupPage />} />
        <Route path="retro" element={<RetroPage />} />
        <Route path="settings" element={<Navigate to="/settings/profile" replace />} />
        <Route path="settings/:tab" element={<SettingsPage />} />
        <Route path="join/:code" element={<JoinSignedInPage email={data.user.email} />} />
        {data.user.isPlatformAdmin && <Route path="admin" element={<AdminPage user={data.user} />} />}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

/** Links in notifications look like /tasks/42; open that task over the board. */
function TaskRedirect() {
  const { number } = useParams()
  return <Navigate to={`/board?task=${number}`} replace />
}

/** Signed in but not (or no longer) in any household. */
function NoHousehold({ data }: { data: Bootstrap }) {
  const user = data.user!
  const signOut = async () => {
    await post('/auth/logout').catch(() => {})
    await clearLocalData()
    location.assign('/')
  }
  return (
    <AuthLayout title={`Hi ${user.displayName}`} subtitle="You're not part of a household yet. Ask a household owner or admin to invite you by email.">
      <div className="space-y-3">
        {(data.households ?? []).map((h) => (
          <Card key={h.id} className="flex items-center justify-between p-3">
            <span className="text-[13px] font-medium">{h.name}</span>
            <Button size="sm" onClick={() => switchHousehold(h.id)}>
              Open
            </Button>
          </Card>
        ))}
        {user.isPlatformAdmin && (
          <Button variant="primary" className="h-10 w-full" icon={<Home className="size-4" />} onClick={() => location.assign('/admin')}>
            Create a household
          </Button>
        )}
        <Button variant="ghost" className="w-full" icon={<LogOut className="size-4" />} onClick={signOut}>
          Sign out
        </Button>
      </div>
    </AuthLayout>
  )
}
