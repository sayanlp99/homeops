# User guide

HomeOps is a household app: chores in weekly sprints, meal planning, groceries,
pantry and budget, shared live between everyone in a household.

- [Getting in](#getting-in)
- [Households and roles](#households-and-roles)
- [Phones and notifications](#phones-and-notifications)
- [Fridge notes](#fridge-notes)
- [Tasks, sprints and chores](#tasks-sprints-and-chores)
- [Meals, recipes, grocery and pantry](#meals-recipes-grocery-and-pantry)
- [Nutrition](#nutrition)
- [Budget](#budget)
- [Retro, insights and kudos](#retro-insights-and-kudos)
- [Offline use](#offline-use)
- [Keyboard shortcuts](#keyboard-shortcuts)

## Getting in

**First person on a new server.** The first visit shows *Set up HomeOps*. Whoever
completes it becomes the platform administrator and owner of the first household.
Do it right after installing: until then anyone who finds the URL could claim it.

**Everyone else joins by email invite.** There is no public sign-up.

1. An owner or admin opens **Settings → Members**, enters the person's email and
   role, and presses **Send invite**.
2. The person gets an email with a link. If email isn't set up on the server, the
   screen shows the link instead: copy it and send it on WhatsApp or similar.
3. Opening the link, they choose a name, username and password. The account gets
   the invited email.

An invite works once, expires after 7 days and only works for the email it was sent
to. Someone who already has an account signs in on the invite page and is added to
the new household. Pending invites can be copied again or revoked from the same page.

**Signing in.** Use your username *or* email with your password, or a passkey
(Face ID, fingerprint, Windows Hello). Add passkeys in **Settings → Sign-in &
security**. Sessions last 90 days and slide while you use the app. The same page
lists your signed-in devices and lets you sign any of them out.

**Forgot your password?** *Forgot password?* on the sign-in page emails a reset link
(valid 1 hour, once). Resetting signs you out everywhere else. This needs email to
be set up on the server; otherwise ask the administrator.

**Changing your email** (Settings → Profile) asks for your current password,
because the email can be used to reset it.

## Households and roles

A person can belong to several households, for example your own home and your
parents' home, each with its own tasks, meals and budget. Switch with the household
name at the top of the sidebar (desktop) or the top bar (phone). Notifications from
another household show as a count there, and tapping a notification opens the
right household.

| Role | Can do |
| --- | --- |
| **Owner** | Everything, including managing admins and other owners, and deleting the household |
| **Admin** | Household settings, invites and members (up to admin) |
| **Member** | Day-to-day use: tasks, meals, grocery, pantry, budget, retro |
| **Viewer** | Look at everything, change nothing. Good for a cook, a guest or a relative. Viewers see a "View only" banner |

Change a member's role or remove them from **Settings → Members**. A household
always keeps at least one owner. Removing someone returns their open tasks to
*unassigned*. Anyone can **leave** a household, and an owner can **delete** it, which
removes all its data for everyone, by typing its name to confirm.

**Platform administrator.** The person who set up the server also sees
**Administration** in the account and household menus. There they can:

- create households, either owning them or naming an owner by email (an existing
  account is added straight away; anyone else gets an owner invite);
- invite anyone into any household, and list every account with its households;
- edit emails and grant administrator rights.

The administrator does **not** see a household's data unless they are a member of it.

## Phones and notifications

Install HomeOps as an app for the best experience:

- **Android (Chrome):** open the site, then menu → *Install app*.
- **iPhone (iOS 16.4+):** open it in **Safari**, Share → *Add to Home Screen*, and
  always open HomeOps from that icon. iOS only allows web notifications for
  installed apps, and Chrome on iPhone cannot receive them at all.
- **Desktop:** Chrome/Edge show an install icon in the address bar. Safari on Mac
  can *Add to Dock*.

Then go to **Settings → Notifications → Enable notifications** on *each* device.
Every browser and installed app is its own device. **Your devices** lists them all,
and **Send test** shows the delivery result for each one. If a device is missing
from the list, enabling didn't reach the server. Enable it again on that device.
Removing an old device there stops duplicate notifications (for example a Safari
tab *and* the Dock app on one Mac).

You get notified about:

- tasks assigned to you;
- an 08:00 digest of today's tasks and a 19:00 nudge for what's still open;
- planning and retro reminders (30 minutes before, and at the start);
- meal prep reminders (e.g. "soak rajma" 10 hours before lunch) and "use soon"
  pantry items at 09:00;
- budget alerts at 80% and 100% of a category, and bill reminders;
- kudos from others;
- a new fridge note from someone else.

Each kind can be switched off per person, and **quiet hours** hold pushes until
they end. The bell icon (inbox) always has everything.

## Fridge notes

The **On the fridge** board at the top of Home is for quick notes to each other,
like sticky notes on a real fridge: "gas booked for Tuesday", "buy curd on the
way back". Tap **+ Note**, write it (up to 500 characters, any language), pick a
colour and optionally **pin** it so it stays first. Tap a note to edit, recolour,
pin or delete it.
Switch a note to **Checklist** for lists like shopping or packing: add a title if
you like, then items (Enter adds the next one, up to 30). Tick items straight on
the fridge without opening the note; both of you can tick at the same time. A
note with several lines turns into a checklist with one item per line, and back.
Under **Take it down** choose *After today*, *After tomorrow*, *In a week* or
*Pick a date*: the note shows through that day (it says "Today only" or
"Until Wed 28 Oct") and comes off the fridge by itself afterwards. *Never* keeps
it until someone deletes it. Everyone in the household sees the same board straight away,
and the others get a notification (switch it off under Settings → Notifications →
*Fridge notes*). Viewers can read notes but not change them.

## Tasks, sprints and chores

- **Board** is the current sprint as a Kanban (To do → In progress → Done). Drag
  cards, or open one for details: assignee, priority, points, labels, due date and
  description. Tasks are numbered `HO-12`, and links like `/tasks/12` open them.
- **Planning** is the backlog plus the next sprint. Drag work into the sprint.
- **Sprints** run one week from the day set in Settings → Household. With *auto-roll*
  on, the server completes the sprint when its week ends, carries unfinished tasks
  over and starts the next one.
- **Recurring chores** repeat daily, weekly (chosen weekdays) or monthly (day N,
  clamped to the month's end), every N periods. Each one is assigned to someone or
  rotates between members. Occurrences are created in the current and next sprint,
  and deleting one doesn't bring it back.
- **Labels** (Settings → Labels) categorise tasks: Cleaning, Bills, Repairs and so on.

## Meals, recipes, grocery and pantry

- **Meal plan:** breakfast, lunch, snacks and dinner for each day, with several dishes
  per slot (dal + sabzi + rice). **Suggest** and **Auto-fill** are rule-based. They
  weigh how long since you last had a dish, how often you cook it, favourites, weekday
  habits ("biryani on Sundays"), the pantry, variety across the week and dishes you
  usually pair. Suggestions get noticeably better after 2–4 months of history.
- **Recipes:** add by hand, or paste a link to import it. Most recipe sites work
  through the structured data they publish for Google. **BongEats** is read from
  its page layout, so ingredient groups like "Marinate the fish" become notes on
  each ingredient. Review the imported draft before saving. The server refuses to
  fetch addresses on your home network.
- **Photos** (recipes and receipts) are shrunk on your phone before upload, like a
  photo sent on WhatsApp: turned upright, at most 2048 px on the long side. They
  look sharp full screen and only soften when zoomed in; a 4–8 MB camera photo
  becomes a few hundred KB, so uploads are quick on mobile data.
- **Grocery list:** built from planned, uncooked meals. Amounts are scaled to
  servings, units converted and summed, pantry stock subtracted, rounded up to
  what you'd buy and grouped by aisle. Move an item to another aisle once and it
  stays there. Ticking items off works offline. **Finish shopping** stocks the pantry
  and can record the bill as an expense. The bin icon on an item deletes it;
  **Delete all** clears the whole list (after a confirmation), cart included.
- **Pantry:** marking a meal cooked deducts its ingredients (undo restores them).
  Low-stock and expiry badges, and one-tap restock to the grocery list.

## Nutrition

**Set up each person** in **Settings → Health & nutrition**: sex, age, height (cm
or ft/in), weight, activity, goal (lose gently, maintain, gain) and foods they don't
eat (fish, prawns, chicken, mutton, egg, dairy). Anyone in the household except
viewers can edit anyone's details, so you can keep each other's up to date. Daily
targets are calculated (Mifflin-St Jeor for calories; vitamins and minerals from
ICMR-NIN 2020), and **Daily targets** lets you type your own, for example on a
dietitian's advice.

**Every recipe** shows calories, protein, carbs, fat, fibre, iron, calcium, vitamins
A, C, B12, folate, zinc, magnesium, potassium and sodium per serving, as a share of
the chosen person's target. Values come from the Indian Food Composition Tables
(IFCT 2017) and USDA data. If an ingredient isn't recognised, the recipe lists it
under *Left out of the estimate*; pick what it is and every recipe using it updates.

**Who eats what.** Each planned dish counts one serving for everyone who eats that
food: fish goes only to the fish-eaters. Tap a dish → **Who's eating…** to set
portions (½ katori rice for one, 2 for another) or mark it a **leftover**, which is
eaten again but kept off the grocery list and pantry.

**Cooking as a task.** Tap a dish → **Add cooking task** to open a new task already
filled in ("Cook …", due that day, with the meal time, servings and a link to the
recipe); pick who's cooking and save. It then shows on the board like any other task.

**Auto-fill follows a Bengali home routine** when the recipes have the right tags
(the *Bengali home kitchen* recipe pack has them):

- breakfast dish; one seasonal fruit per person;
- one dairy item a day for calcium, a different one each day: curd or ghol
  (buttermilk) with lunch, or a glass of milk at breakfast;
- lunch of rice + dal + vegetable + protein. Anyone who doesn't eat that day's
  protein (fish) gets their own;
- Wednesday is a quick one-pot dal-with-vegetables plus a bhaja;
- Saturday lunch is khichdi when due (twice a month, with palong shaak ghonto once a
  month);
- Sunday lunch is the weekend special (pulao set) every two weeks; prawns at most
  weekly;
- dinners are chapati + a simple curry, a special on Friday and eating out on
  Saturday;
- **cook once, eat twice**: breakfast, lunch and dinner cooked one day come back
  reheated at the same meal the next day, then something fresh is cooked. The cooking
  day's servings (and the grocery list) cover both days; the second day is marked
  *leftover*, so it isn't bought or taken from the pantry again. Rotis are made
  fresh each night from dough made once for both days (the first night says *Make
  dough for two days*). Fruit and dairy are fresh daily. A special dinner or a meal out is one night only, so the dinner before
  it is cooked for one night.

This is only a starting point: swap any meal for leftovers (Who's eating… →
leftover) or a fresh dish as the fridge allows.

Seasonal dishes (echor in summer, peyajkoli in winter) are only planned in season.
Each pick favours what closes the week's nutrient gaps, and each person's portions
are then sized to their calorie target: rice/chapati first, then fried sides, then
dal and vegetables, protein last.

Salt is counted from each recipe's ingredient list, so set the salt in a recipe to
what you actually use for that quantity and the sodium figures follow.

**Kitchen → Nutrition** shows each person's week: daily average against targets, a
calories-by-day chart, vitamins and minerals, and what to balance with recipes you
already cook. Days with a meal out are left out of averages. These are estimates,
not medical advice.

## Budget

- **Categories** have a monthly limit. You can override any single month and
  optionally roll unspent money over. *Shared* categories are the household budget.
  *Personal* categories, accounts, transactions and bills are visible **only to their
  owner**, which the server enforces in every query.
- **Add an expense** in three taps (the `e` key or the + button on budget pages),
  optionally with a receipt photo.
- **Bank statement import:** CSV, XLSX, the HTML ".xls" many Indian banks serve,
  and HDFC's fixed-width text statement. Columns are detected for HDFC, ICICI, SBI,
  Axis and card layouts, with a manual mapper as fallback that is remembered per
  account. HDFC text statements are checked against the bank's own totals.
  Duplicate rows across overlapping imports are skipped. Transfers between your own
  accounts, card bills, CRED, NPS and SIPs are kept out of the budget. Fixing a
  category teaches a rule for next time, and every import can be undone.
- **Grocery invoices:** upload the Zepto or Blinkit order PDF. The total (summed
  across Blinkit's merged seller invoices), items, pack sizes and fees are read, and
  the PDF is kept as the receipt. When the bank statement arrives later, the matching
  UPI payment is recognised and skipped.
- **Bills:** due day, usual amount and reminder lead time. *Mark paid* records the
  expense and stops that month's reminders.

## Retro, insights and kudos

- **Retro** (one per sprint): while collecting, each person sees only their own
  cards. Reveal together, vote, and give action items an owner. Closing the retro
  creates the actions as tasks in the next sprint, with voted ones as high priority.
- **Insights** (Sprints page): a burndown against an ideal line, velocity (planned
  vs done) and fairness (who completed what). Tasks without points count as 1.
- **Streaks** count consecutive days with at least one task done. **Kudos** can be
  sent from a task someone finished, or from the Retro page.

## Offline use

The app opens instantly from the device's cache, even with no connection. Changes
made offline are queued and sent when you reconnect, even if the app was closed in
between. An amber bar shows while offline. Switching household waits until queued
changes have synced.

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `Ctrl` / `⌘` + `K`, or `/` | Search and jump anywhere |
| `c` | New task |
| `e` | New expense |
