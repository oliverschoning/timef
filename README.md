# timef

Unofficial CLI for [go.poweroffice.net](https://go.poweroffice.net) time tracking.
Reads your existing browser session — no separate auth.

Norwegian "timeføring" (time entry). Built for consultants/employees who want
to log hours, sick days, ferie, and helligdag from the terminal instead of
clicking through the ExtJS web UI.

## How it works

1. Log in to go.poweroffice.net via your default browser (Brave, Chrome, Firefox…) — normal flow.
2. `timef` reads the session cookie directly from your browser's cookie store via [kooky](https://github.com/browserutils/kooky).
3. Every CLI call sends fresh cookies + required PowerOffice headers to the real HTTPS API. No daemon, no scrape, no headless browser.

## Platform support

| OS | Cookie store backend | Tested |
|----|---------------------|--------|
| Linux | libsecret (Gnome Keyring / KWallet must be running) | ✅ Brave |
| macOS | Keychain (auto-prompts on first read) | ⚠️ Should work, untested |
| Windows | DPAPI | ⚠️ May fail on Chrome ≥ 127 due to app-bound encryption — see [kooky issues](https://github.com/browserutils/kooky/issues) |

Works with any Chromium-based browser (Brave, Chrome, Edge, Vivaldi, Opera),
Firefox, and Safari (macOS).

## Status

Hobby project. Works on Linux + Brave for the author. Patches welcome.

Tested:
- Reading week timesheet
- Setting / clearing time entries on billable subprojects
- Listing subprojects
- Logging sick (egenmelding + sykemelding), ferie, lege, permisjon, child sickness via the leave-approval flow
- Logging helligdag + other internal activities via the saveEntry flow

## Install

```bash
go install github.com/oliverschoning/timef/cmd/timef@latest
```

Or from source:

```bash
git clone https://github.com/oliverschoning/timef
cd timef
go install ./cmd/timef
```

## Setup

One-time: write your `goclientid` (PowerOffice tenant ID) so `timef` knows
which client to query. PowerOffice issues this per company.

To find it: open `go.poweroffice.net` in your browser, open DevTools → Network
tab → reload → click any `/api/*` request → look in Request Headers for
`goclientid`. Then:

```bash
mkdir -p ~/.config/timef
echo "<your-goclientid>" > ~/.config/timef/client_id
# or via env var: export TIMEF_CLIENT_ID=<your-goclientid>
```

## Usage

### Auth

```bash
timef login    # opens go.poweroffice.net in your default browser
timef status   # verify session cookies still valid
```

If a call returns 401/403, your session expired — open the browser and log in again, then retry.

### View timesheet

```bash
timef week                                # current ISO week
timef week 2026-05-15                     # week containing date
timef week 2026-04-01 2026-05-27          # all weeks in range (inclusive)
timef week --json                         # raw JSON
timef week 2026-04-01 2026-05-27 --json   # range as JSON array
```

Output: one table per week with day-of-month headers, holiday markers
(Norwegian public holidays), and footer showing Required / Flextime balance.
External comments wrap under each cell.

### Browse projects

```bash
timef projects                            # active billable subprojects
timef projects konsesjon                  # case-insensitive substring filter
timef projects --all                      # +inactive +internal
timef projects --json
```

Returns `ID | Customer | Project / Subproject | Code`. Use the ID with `timef set`.

### Log time on a billable project

```bash
timef set <subprojectId> <date> <h:mm|minutes> [comment]
timef clear <subprojectId> <date>
```

Examples:

```bash
timef set 58757091 2026-05-27 7:30 "utvikling + standup"
timef set 58757091 2026-05-27 450 "minutes also accepted"
timef clear 58757091 2026-05-27
```

The CLI errors out if the project requires an external comment and you didn't supply one.

### Log leave / internal activities

```bash
timef leave                                       # list all available codes
timef leave <code|alias> <date> [h:mm] [comment]  # default 7:30, single day
```

Examples:

```bash
timef leave sick 2026-05-28                       # egenmelding (alias for code 902)
timef leave sick 2026-05-28 4:00 "halv dag"
timef leave doctor 2026-05-28                     # sykemelding (903)
timef leave ferie 2026-05-28                      # vacation (907)
timef leave lege 2026-05-28 2:00                  # doctor's appt (900)
timef leave permisjon 2026-05-28 7:30 "begravelse"
timef leave helligdag 2026-05-28                  # paid holiday (908)
timef leave 930 2026-05-28 7:30 "kurs i Go"       # internal "Kurs" by code
```

Two flows under the hood, picked automatically based on the activity's
`holidayLeaveType`:

* **submitforapproval** — sick, ferie, lege, permisjon, child sickness, militær, velferd. Goes through approval (often auto-approved).
* **saveEntry** — helligdag and internal non-billable activities (Kurs, Salg, Møteaktivitet, etc.).

For the saveEntry path, `timef` auto-discovers your employer's internal
project + customer + department by looking up the most recent line in your
timesheet that uses the same activity. So you need to have logged that
activity at least once via the UI before `timef` can log it for you.

#### Aliases

`sick` `sick-fast` `egenmelding` `doctor` `sykemelding` `child` `child-fast`
`barn` `barns-sykdom` `ferie` `vacation` `holiday` `helligdag`
`helligdag-ub` `lege` `dentist` `permisjon` `permisjon-ub` `velferd`
`militær`. Otherwise pass the raw activity code (e.g. `902`).

The split between `sick` (902, timelønn) and `sick-fast` (914, fastlønn) and
between `child` (904) and `child-fast` (916) reflects the PowerOffice
activity catalog. Consultants on hourly billing use `sick` / `child`;
salaried employees use the `-fast` variants.

## Project layout

```
cmd/timef/         main CLI
cmd/timef-debug/   raw GET helper for poking API endpoints (diagnostic)
internal/session/  HTTP client + cookie loader (kooky)
internal/holiday/  Norwegian public holiday calculation (Easter + fixed dates)
```

## Caveats

* Not affiliated with PowerOffice. Uses undocumented endpoints from the web UI.
  They can break any release.
* Only tested with one tenant. Multi-tenant users would need to switch
  `client_id` between calls — not implemented.
* The leave-approval flow guesses the payload shape for some activities based
  on a small set of captures. If a leave type fails, please open an issue
  with the network capture.
* `cmd/timef-debug` lets you GET any URL through the session. Useful for
  exploring endpoints, but treat it as a diagnostic tool — it'll happily
  return arbitrary data.

## License

MIT — see [LICENSE](LICENSE).
