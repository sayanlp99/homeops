import { ChefHat, RotateCcw, Wallet, WifiOff } from 'lucide-react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { AppShell } from '@/components/AppShell'
import { Button, Spinner } from '@/components/ui'
import { JoinPage, LoginPage, SetupPage } from '@/features/auth/AuthPages'
import { HomePage } from '@/features/home/HomePage'
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
        <Route
          path="meals"
          element={<ComingSoon title="Meals" phase={2} icon={<ChefHat className="size-5" />} items={['Weekly planner: breakfast, lunch, snacks, dinner', 'Recipe library with photos & URL import', 'Auto grocery list & pantry tracking', 'Mix-and-match suggestions from your history']} />}
        />
        <Route
          path="budget"
          element={<ComingSoon title="Budget" phase={3} icon={<Wallet className="size-5" />} items={['Monthly category envelopes, shared + personal', '3-tap expense entry with receipt photos', 'HDFC / ICICI / SBI / Axis / card statement import', '80% and 100% budget alerts']} />}
        />
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
