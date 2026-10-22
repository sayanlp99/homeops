import { RotateCcw, WifiOff } from 'lucide-react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { AppShell } from '@/components/AppShell'
import { Button, Spinner } from '@/components/ui'
import { JoinPage, LoginPage, SetupPage } from '@/features/auth/AuthPages'
import { GroceryPage } from '@/features/grocery/GroceryPage'
import { HomePage } from '@/features/home/HomePage'
import { MealPlanPage } from '@/features/meals/MealPlanPage'
import { BillsPage } from '@/features/money/BillsPage'
import { BudgetPage } from '@/features/money/BudgetPage'
import { ImportPage } from '@/features/money/ImportPage'
import { MoneySetupPage } from '@/features/money/MoneySetupPage'
import { TransactionsPage } from '@/features/money/TransactionsPage'
import { PantryPage } from '@/features/pantry/PantryPage'
import { RecipeDetailPage } from '@/features/recipes/RecipeDetailPage'
import { RecipeEditorPage } from '@/features/recipes/RecipeEditorPage'
import { RecipesPage } from '@/features/recipes/RecipesPage'
import { ComingSoon } from '@/features/placeholder/ComingSoon'
import { RecurringPage } from '@/features/recurring/RecurringPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { SprintsPage } from '@/features/sprints/SprintsPage'
import { BacklogPage } from '@/features/tasks/BacklogPage'
import { BoardPage } from '@/features/tasks/BoardPage'
import { useBootstrap } from '@/lib/session'

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
        <Route path="*" element={data.setupRequired ? <SetupPage /> : <LoginPage />} />
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
        <Route path="budget" element={<BudgetPage />} />
        <Route path="budget/transactions" element={<TransactionsPage />} />
        <Route path="budget/bills" element={<BillsPage />} />
        <Route path="budget/import" element={<ImportPage />} />
        <Route path="budget/setup" element={<MoneySetupPage />} />
        <Route
          path="retro"
          element={<ComingSoon title="Retro" phase={4} icon={<RotateCcw className="size-5" />} items={['Went well / Improve / Actions with private reveal & voting', 'Action items become next sprint’s tasks', 'Burndown, velocity & fairness charts', 'Kudos and streaks']} />}
        />
        <Route path="settings" element={<Navigate to="/settings/profile" replace />} />
        <Route path="settings/:tab" element={<SettingsPage />} />
        <Route path="join/:code" element={<Navigate to="/" replace />} />
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
