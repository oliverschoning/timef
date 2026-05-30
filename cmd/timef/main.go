package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/oliverschoning/timef/internal/holiday"
	"github.com/oliverschoning/timef/internal/session"
)

var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	bold   = lipgloss.NewStyle().Bold(true)
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "login":
		cmdLogin()
	case "status":
		cmdStatus()
	case "set":
		cmdSet(os.Args[2:])
	case "clear":
		cmdClear(os.Args[2:])
	case "week":
		cmdWeek(os.Args[2:])
	case "projects":
		cmdProjects(os.Args[2:])
	case "leave":
		cmdLeave(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: timef <command>")
	fmt.Println("  login                            Open PowerOffice Go in default browser to log in")
	fmt.Println("  status                           Verify session cookies work")
	fmt.Println("  week                             Current week")
	fmt.Println("  week YYYY-MM-DD                  Week containing date")
	fmt.Println("  week YYYY-MM-DD YYYY-MM-DD       All weeks between dates (inclusive)")
	fmt.Println("  set <subprojectId> <date> <h:mm|minutes> [comment]   Set time entry")
	fmt.Println("  clear <subprojectId> <date>                           Clear time entry (set to 0)")
	fmt.Println("  projects [search]                Active billable subprojects (filter by substring)")
	fmt.Println("  projects --all                   Include inactive + internal")
	fmt.Println("  leave                            List available leave activities (sick, ferie, helligdag…)")
	fmt.Println("  leave <code|alias> <date> [h:mm] [comment]   Log leave (default 7:30)")
	fmt.Println("  (add --json to projects or week for raw JSON)")
}

func cmdLogin() {
	if err := exec.Command("xdg-open", session.BaseURL).Start(); err != nil {
		fail(fmt.Errorf("could not open browser: %w", err))
	}
	fmt.Println(yellow.Render("Opened " + session.BaseURL + " in your default browser."))

	if session.HasGoSession() {
		fmt.Println(green.Render("✓ Session cookie already present. Run: timef status"))
		return
	}

	fmt.Print(dim.Render("Waiting for login (browser flushes cookies to disk periodically)…"))
	// Browser buffers freshly-set cookies in memory; kooky reads the on-disk
	// store. Poll until the BFF session cookie lands, or time out.
	const timeout = 90 * time.Second
	const interval = 2 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if session.HasGoSession() {
			fmt.Println()
			fmt.Println(green.Render("✓ Logged in. Run: timef status"))
			return
		}
		time.Sleep(interval)
		fmt.Print(dim.Render("."))
	}
	fmt.Println()
	fmt.Println(yellow.Render("Timed out waiting for session cookie."))
	fmt.Println(dim.Render("If you logged in, give the browser a moment to save cookies, then run: timef status"))
}

func cmdStatus() {
	c, err := session.NewClient()
	if err != nil {
		fail(err)
	}
	now := time.Now()
	year, week := now.ISOWeek()
	url := fmt.Sprintf("/api/timetracking/timesheet/timesheet/week?weekNo=%d&year=%d", week, year)
	if _, err := c.Do("GET", url, nil); err != nil {
		fail(err)
	}
	fmt.Println(green.Render("✓ Logged in, session cookies valid."))
}

type dayEntry struct {
	Minutes         int    `json:"minutes"`
	Date            string `json:"date"`
	ExternalComment string `json:"externalComment"`
}

type dayTotal struct {
	WorkMinutes int `json:"workMinutes"`
}

type line struct {
	CustomerName   string    `json:"customerName"`
	SubprojectName string    `json:"subprojectName"`
	SubprojectID   int       `json:"subprojectId"`
	ActivityName   string    `json:"activityName"`
	ActivityID     int       `json:"activityId"`
	SumInMinutes   int       `json:"sumInMinutes"`
	RowStyle       int       `json:"rowStyle"`
	RowTitle       string    `json:"rowTitle"`
	MonRaw         *dayEntry `json:"mon"`
	TueRaw         *dayEntry `json:"tue"`
	WedRaw         *dayEntry `json:"wed"`
	ThuRaw         *dayEntry `json:"thu"`
	FriRaw         *dayEntry `json:"fri"`
	SatRaw         *dayEntry `json:"sat"`
	SunRaw         *dayEntry `json:"sun"`
}

type week struct {
	Data struct {
		WeekNo              int    `json:"weekNo"`
		Year                int    `json:"year"`
		EmployeeName        string `json:"employeeName"`
		EmployeeID          int64  `json:"employeeId"`
		DefaultDepartmentID int64  `json:"defaultDepartmentId"`
		Mon          dayTotal `json:"mon"`
		Tue          dayTotal `json:"tue"`
		Wed          dayTotal `json:"wed"`
		Thu          dayTotal `json:"thu"`
		Fri          dayTotal `json:"fri"`
		Sat          dayTotal `json:"sat"`
		Sun          dayTotal `json:"sun"`
		Lines        []line   `json:"lines"`
	} `json:"data"`
}

type weekKey struct {
	year, week int
}

func cmdWeek(args []string) {
	raw := false
	dates := []string{}
	for _, a := range args {
		if a == "--json" {
			raw = true
			continue
		}
		dates = append(dates, a)
	}

	weeks, err := resolveWeeks(dates)
	if err != nil {
		fail(err)
	}

	c, err := session.NewClient()
	if err != nil {
		fail(err)
	}

	if raw {
		blobs := make([]json.RawMessage, 0, len(weeks))
		for _, wk := range weeks {
			data, err := fetchWeek(c, wk.year, wk.week)
			if err != nil {
				fail(err)
			}
			blobs = append(blobs, data)
		}
		if len(blobs) == 1 {
			fmt.Println(prettyJSON(blobs[0]))
			return
		}
		out, _ := json.MarshalIndent(blobs, "", "  ")
		fmt.Println(string(out))
		return
	}

	for i, wk := range weeks {
		if i > 0 {
			fmt.Println()
		}
		data, err := fetchWeek(c, wk.year, wk.week)
		if err != nil {
			fail(err)
		}
		var w week
		if err := json.Unmarshal(data, &w); err != nil {
			fail(fmt.Errorf("parse: %w", err))
		}
		renderWeek(&w)
	}
}

func fetchWeek(c *session.Client, year, weekNo int) ([]byte, error) {
	url := fmt.Sprintf("/api/timetracking/timesheet/timesheet/week?weekNo=%d&year=%d", weekNo, year)
	return c.Do("GET", url, nil)
}

func prettyJSON(b []byte) string {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

func resolveWeeks(dates []string) ([]weekKey, error) {
	switch len(dates) {
	case 0:
		y, w := time.Now().ISOWeek()
		return []weekKey{{y, w}}, nil
	case 1:
		t, err := time.Parse("2006-01-02", dates[0])
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w (expected YYYY-MM-DD)", dates[0], err)
		}
		y, w := t.ISOWeek()
		return []weekKey{{y, w}}, nil
	case 2:
		from, err := time.Parse("2006-01-02", dates[0])
		if err != nil {
			return nil, fmt.Errorf("bad from date %q: %w", dates[0], err)
		}
		to, err := time.Parse("2006-01-02", dates[1])
		if err != nil {
			return nil, fmt.Errorf("bad to date %q: %w", dates[1], err)
		}
		if to.Before(from) {
			return nil, fmt.Errorf("to date %s is before from date %s", dates[1], dates[0])
		}
		seen := map[weekKey]bool{}
		out := []weekKey{}
		for t := from; !t.After(to); t = t.AddDate(0, 0, 1) {
			y, w := t.ISOWeek()
			k := weekKey{y, w}
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("too many date args (max 2)")
	}
}

func renderWeek(w *week) {
	d := &w.Data
	monday := mondayOfISOWeek(d.Year, d.WeekNo)
	sunday := monday.AddDate(0, 0, 6)
	fmt.Println(bold.Render(fmt.Sprintf("Week %d — %s – %s — %s",
		d.WeekNo,
		monday.Format("2006-01-02"),
		sunday.Format("2006-01-02"),
		d.EmployeeName,
	)))

	weekday := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	dayLabels := [7]string{}
	dayHolidays := [7]string{}
	for i := 0; i < 7; i++ {
		dt := monday.AddDate(0, 0, i)
		label := fmt.Sprintf("%s %02d", weekday[i], dt.Day())
		if name := holiday.Lookup(dt); name != "" {
			dayHolidays[i] = name
			label = red.Render(label + "*")
		}
		dayLabels[i] = label
	}

	headers := []string{
		"ID",
		"Project / Activity",
		dayLabels[0], dayLabels[1], dayLabels[2], dayLabels[3],
		dayLabels[4], dayLabels[5], dayLabels[6],
		"Sum",
	}

	rows := [][]string{}
	var dayTot [7]int
	totalSum := 0
	for _, ln := range d.Lines {
		if ln.RowStyle != 0 {
			continue
		}
		if ln.SumInMinutes == 0 && allNil(ln.MonRaw, ln.TueRaw, ln.WedRaw, ln.ThuRaw, ln.FriRaw, ln.SatRaw, ln.SunRaw) {
			continue
		}
		days := [7]*dayEntry{ln.MonRaw, ln.TueRaw, ln.WedRaw, ln.ThuRaw, ln.FriRaw, ln.SatRaw, ln.SunRaw}
		for i, dy := range days {
			if dy != nil {
				dayTot[i] += dy.Minutes
			}
		}
		totalSum += ln.SumInMinutes
		idStr := "—"
		if ln.SubprojectID != 0 {
			idStr = fmt.Sprintf("%d", ln.SubprojectID)
		}
		rows = append(rows, []string{
			idStr,
			projectLabel(ln),
			fmtDay(ln.MonRaw),
			fmtDay(ln.TueRaw),
			fmtDay(ln.WedRaw),
			fmtDay(ln.ThuRaw),
			fmtDay(ln.FriRaw),
			fmtDay(ln.SatRaw),
			fmtDay(ln.SunRaw),
			fmtMinutes(ln.SumInMinutes),
		})
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dim).
		BorderRow(true).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return bold.Padding(0, 1)
			}
			if col == 0 {
				return dim.Padding(0, 1).Align(lipgloss.Right)
			}
			if col == 1 {
				return lipgloss.NewStyle().Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1).Align(lipgloss.Right)
		})

	fmt.Println(t.Render())

	totalRow := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dim).
		Headers("Total", dayLabels[0], dayLabels[1], dayLabels[2], dayLabels[3], dayLabels[4], dayLabels[5], dayLabels[6], "Sum").
		Rows([]string{
			bold.Render("Logged"),
			fmtMinutes(dayTot[0]),
			fmtMinutes(dayTot[1]),
			fmtMinutes(dayTot[2]),
			fmtMinutes(dayTot[3]),
			fmtMinutes(dayTot[4]),
			fmtMinutes(dayTot[5]),
			fmtMinutes(dayTot[6]),
			bold.Render(fmtMinutes(totalSum)),
		}).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return bold.Padding(0, 1)
			}
			if col == 0 {
				return lipgloss.NewStyle().Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1).Align(lipgloss.Right)
		})
	fmt.Println(totalRow.Render())

	for i, name := range dayHolidays {
		if name == "" {
			continue
		}
		dt := monday.AddDate(0, 0, i)
		fmt.Printf("  %s %s %s — %s\n",
			dim.Render("Holiday:"),
			dayLabels[i],
			dim.Render(dt.Format("Jan")),
			red.Render(name),
		)
	}

	// Holidays still require logging (Helligdag - betalt). API subtracts
	// weekday holidays from required, so add them back for the full expected.
	weekdayHolidays := 0
	for i := 0; i < 5; i++ {
		if dayHolidays[i] != "" {
			weekdayHolidays++
		}
	}

	for _, ln := range d.Lines {
		switch ln.RowTitle {
		case "Normal Time":
			required := ln.SumInMinutes + weekdayHolidays*450
			fmt.Printf("  %s %s   %s %s\n",
				dim.Render("Required:"), fmtMinutes(required),
				dim.Render("Logged:"), fmtMinutes(totalSum))
		case "Flextime Balance":
			val := fmtMinutes(ln.SumInMinutes)
			if ln.SumInMinutes < 0 {
				val = red.Render(val)
			} else if ln.SumInMinutes > 0 {
				val = green.Render(val)
			}
			fmt.Printf("  %s %s\n", dim.Render("Flextime balance:"), val)
		}
	}
}

func projectLabel(ln line) string {
	parts := []string{}
	if ln.CustomerName != "" {
		parts = append(parts, ln.CustomerName)
	}
	if ln.SubprojectName != "" {
		parts = append(parts, ln.SubprojectName)
	}
	if ln.ActivityName != "" {
		parts = append(parts, ln.ActivityName)
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " / "
		}
		out += p
	}
	return out
}

func fmtDay(d *dayEntry) string {
	if d == nil || d.Minutes == 0 {
		return dim.Render("·")
	}
	hours := fmtMinutes(d.Minutes)
	cmt := strings.TrimSpace(d.ExternalComment)
	if cmt == "" {
		return hours
	}
	return hours + "\n" + dim.Render(wordWrap(cmt, 16))
}

// wordWrap breaks s into lines of at most width chars, splitting on spaces.
// A single word longer than width gets its own (over-long) line.
func wordWrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len([]rune(cur))+1+len([]rune(w)) > width {
			lines = append(lines, cur)
			cur = w
		} else {
			cur += " " + w
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}

func fmtMinutes(m int) string {
	if m == 0 {
		return dim.Render("0:00")
	}
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	return fmt.Sprintf("%s%d:%02d", sign, m/60, m%60)
}

// mondayOfISOWeek returns the Monday date for the given ISO year/week.
func mondayOfISOWeek(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mondayWeek1 := jan4.AddDate(0, 0, 1-wd)
	return mondayWeek1.AddDate(0, 0, (week-1)*7)
}

func allNil(es ...*dayEntry) bool {
	for _, e := range es {
		if e != nil {
			return false
		}
	}
	return true
}

var weekdayFields = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// weekdayField returns the JSON field name for a given date (mon..sun).
func weekdayField(t time.Time) string {
	// Go weekday: Sunday=0, Monday=1..Saturday=6
	// Map: Monday→mon (idx 0), Sunday→sun (idx 6)
	return weekdayFields[(int(t.Weekday())+6)%7]
}

// parseHMM parses "h:mm" (e.g. "7:30") or raw minutes (e.g. "450").
func parseHMM(s string) (int, error) {
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid time %q (expected h:mm or minutes)", s)
		}
		if m >= 60 || m < 0 {
			return 0, fmt.Errorf("invalid minutes in %q", s)
		}
		return h*60 + m, nil
	}
	m, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q (expected h:mm or minutes)", s)
	}
	return m, nil
}

// strOrEmpty returns string form of a JSON-decoded id value (json.Number, string, or nil).
func strOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func cmdSet(args []string) {
	if len(args) < 3 {
		fail(fmt.Errorf("usage: timef set <subprojectId> <date> <h:mm|minutes> [comment]"))
	}

	subprojectID, err := strconv.Atoi(args[0])
	if err != nil {
		fail(fmt.Errorf("invalid subprojectId: %w", err))
	}

	date, err := time.Parse("2006-01-02", args[1])
	if err != nil {
		fail(fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", args[1], err))
	}

	minutes, err := parseHMM(args[2])
	if err != nil {
		fail(err)
	}

	comment := ""
	if len(args) > 3 {
		comment = strings.Join(args[3:], " ")
	}

	c, err := session.NewClient()
	if err != nil {
		fail(err)
	}

	year, weekNo := date.ISOWeek()
	raw, err := fetchWeek(c, year, weekNo)
	if err != nil {
		fail(err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var resp map[string]any
	if err := dec.Decode(&resp); err != nil {
		fail(fmt.Errorf("parse week response: %w", err))
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		fail(fmt.Errorf("unexpected response shape: no data key"))
	}

	linesRaw, ok := data["lines"]
	if !ok {
		fail(fmt.Errorf("unexpected response shape: no lines"))
	}
	lines, _ := linesRaw.([]any)

	dayField := weekdayField(date)
	var matchedLine map[string]any
	for _, l := range lines {
		ln, ok := l.(map[string]any)
		if !ok {
			continue
		}
		sidRaw := ln["subprojectId"]
		if sidRaw == nil {
			continue
		}
		sid, err := strconv.Atoi(strOrEmpty(sidRaw))
		if err != nil || sid != subprojectID {
			continue
		}
		matchedLine = ln
		break
	}

	if matchedLine == nil {
		fail(fmt.Errorf("subproject %d not found in week %d — add via UI first to create the line", subprojectID, weekNo))
	}

	// requireExternalComment validation (server rejects blank comment if true).
	if rec, _ := matchedLine["requireExternalComment"].(bool); rec && minutes > 0 && strings.TrimSpace(comment) == "" {
		fail(fmt.Errorf("this project requires an external comment — pass one as last arg"))
	}

	// Existing day entry id (string form). Empty for new entry.
	entryID := ""
	if dayRaw, ok := matchedLine[dayField].(map[string]any); ok && dayRaw != nil {
		entryID = strOrEmpty(dayRaw["id"])
	}

	payload := map[string]any{
		"id":                    entryID,
		"customerId":            strOrEmpty(matchedLine["customerId"]),
		"activityId":            strOrEmpty(matchedLine["activityId"]),
		"projectId":             strOrEmpty(matchedLine["projectId"]),
		"departmentId":          strOrEmpty(matchedLine["departmentId"]),
		"timeSpecificationId":   matchedLine["timeSpecificationId"], // can be null
		"date":                  date.Format("2006-01-02"),
		"minutes":               minutes,
		"externalComment":       comment,
		"internalComment":       "",
		"isTimeOff":             false,
		"assignmentAgreementId": strOrEmpty(matchedLine["assignmentAgreementId"]),
		"qualityDeliveryAreaId": strOrEmpty(matchedLine["qualityDeliveryAreaId"]),
	}

	if _, err := c.Do("PUT", "/api/timetracking/timesheet/timesheet/saveEntry", payload); err != nil {
		fail(err)
	}

	display := fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
	if minutes == 0 {
		display = "cleared"
	}
	fmt.Println(green.Render(fmt.Sprintf("✓ %s → %s on subproject %d", date.Format("Mon 2006-01-02"), display, subprojectID)))
}

type project struct {
	ID             int    `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	FullName       string `json:"fullName"`
	SubprojectName string `json:"subprojectName"`
	CustomerName   string `json:"customerName"`
	IsActive       bool   `json:"isActive"`
	IsBillable     bool   `json:"isBillable"`
	IsInternal     bool   `json:"isInternal"`
	IsSubproject   bool   `json:"isSubproject"`
}

type projectsResp struct {
	Data []project `json:"data"`
}

func cmdProjects(args []string) {
	all := false
	raw := false
	search := ""
	for _, a := range args {
		switch {
		case a == "--all":
			all = true
		case a == "--json":
			raw = true
		case strings.HasPrefix(a, "--"):
			fail(fmt.Errorf("unknown flag: %s", a))
		default:
			search = a
		}
	}

	c, err := session.NewClient()
	if err != nil {
		fail(err)
	}
	data, err := c.Do("GET", "/api/cache/cachedentities/projects?language=en-US", nil)
	if err != nil {
		fail(err)
	}

	if raw {
		fmt.Println(prettyJSON(data))
		return
	}

	var resp projectsResp
	if err := json.Unmarshal(data, &resp); err != nil {
		fail(fmt.Errorf("parse: %w", err))
	}

	needle := strings.ToLower(search)
	rows := [][]string{}
	for _, p := range resp.Data {
		if p.ID <= 0 {
			continue
		}
		if !p.IsSubproject {
			continue
		}
		if !all && (!p.IsActive || !p.IsBillable || p.IsInternal) {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(p.FullName + " " + p.CustomerName)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", p.ID),
			p.CustomerName,
			projectDisplayName(p),
			p.Code,
		})
	}

	if len(rows) == 0 {
		if search != "" {
			fmt.Println(yellow.Render(fmt.Sprintf("No subprojects match %q", search)))
		} else {
			fmt.Println(yellow.Render("No subprojects found"))
		}
		return
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dim).
		Headers("ID", "Customer", "Project / Subproject", "Code").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return bold.Padding(0, 1)
			}
			if col == 0 {
				return dim.Padding(0, 1).Align(lipgloss.Right)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	fmt.Println(t.Render())
	fmt.Println(dim.Render(fmt.Sprintf("  %d subprojects", len(rows))))
}

type activity struct {
	ID                  int64  `json:"id"`
	Code                string `json:"code"`
	Name                string `json:"name"`
	Type                int    `json:"type"`
	HolidayLeaveType    *int   `json:"holidayLeaveType"`
	TimeSpecificationID *int64 `json:"timeSpecificationId"`
	IsActive            bool   `json:"isActive"`
}

type activitiesResp struct {
	Data []activity `json:"data"`
}

func fetchActivities(c *session.Client) ([]activity, error) {
	data, err := c.Do("GET", "/api/cache/cachedentities/activities?language=en-US", nil)
	if err != nil {
		return nil, err
	}
	var resp activitiesResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// leaveAliases maps user-friendly names to activity codes.
var leaveAliases = map[string]string{
	"sick":         "902",
	"sick-fast":    "914",
	"egenmelding":  "902",
	"doctor":       "903",
	"sykemelding":  "903",
	"child":        "904",
	"child-fast":   "916",
	"barn":         "904",
	"barns-sykdom": "904",
	"ferie":        "907",
	"vacation":     "907",
	"holiday":      "908",
	"helligdag":    "908",
	"helligdag-ub": "909",
	"lege":         "900",
	"dentist":      "900",
	"permisjon":    "901",
	"permisjon-ub": "905",
	"velferd":      "913",
	"militær":      "906",
}

// findLeaveActivity resolves a code or alias to an activity (only "leave-like" ones).
func findLeaveActivity(acts []activity, query string) *activity {
	if code, ok := leaveAliases[strings.ToLower(query)]; ok {
		query = code
	}
	for i, a := range acts {
		if !a.IsActive {
			continue
		}
		// Show only absence/internal/leave activities (skip pure billable work)
		isLeaveLike := a.HolidayLeaveType != nil || a.Type == 250
		if !isLeaveLike {
			continue
		}
		if a.Code == query || fmt.Sprintf("%d", a.ID) == query || strings.EqualFold(a.Name, query) {
			return &acts[i]
		}
	}
	return nil
}

func cmdLeave(args []string) {
	c, err := session.NewClient()
	if err != nil {
		fail(err)
	}

	acts, err := fetchActivities(c)
	if err != nil {
		fail(fmt.Errorf("fetch activities: %w", err))
	}

	if len(args) == 0 {
		listLeaveActivities(acts)
		return
	}

	if len(args) < 2 {
		fail(fmt.Errorf("usage: timef leave <code|alias> <date> [h:mm|minutes] [comment]"))
	}

	act := findLeaveActivity(acts, args[0])
	if act == nil {
		fail(fmt.Errorf("no leave activity matching %q — run `timef leave` to see options", args[0]))
	}

	date, err := time.Parse("2006-01-02", args[1])
	if err != nil {
		fail(fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", args[1], err))
	}

	minutes := 450 // default 7:30
	if len(args) > 2 {
		m, perr := parseHMM(args[2])
		if perr != nil {
			fail(perr)
		}
		minutes = m
	}

	comment := ""
	if len(args) > 3 {
		comment = strings.Join(args[3:], " ")
	}

	if act.HolidayLeaveType != nil {
		submitLeaveApproval(c, act, date, minutes, comment)
	} else {
		submitInternalEntry(c, act, date, minutes, comment, acts)
	}
}

func listLeaveActivities(acts []activity) {
	rows := [][]string{}
	for _, a := range acts {
		if !a.IsActive {
			continue
		}
		isLeaveLike := a.HolidayLeaveType != nil || a.Type == 250
		if !isLeaveLike {
			continue
		}
		flow := "internal"
		if a.HolidayLeaveType != nil {
			flow = fmt.Sprintf("leave (hlt=%d)", *a.HolidayLeaveType)
		}
		rows = append(rows, []string{a.Code, a.Name, flow})
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dim).
		Headers("Code", "Name", "Flow").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return bold.Padding(0, 1)
			}
			if col == 0 {
				return dim.Padding(0, 1).Align(lipgloss.Right)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	fmt.Println(t.Render())
	fmt.Println(dim.Render("  Aliases: sick, doctor, ferie, holiday, lege, permisjon, velferd, child"))
}

// submitLeaveApproval calls /api/holidayandleave/.../submitforapproval.
func submitLeaveApproval(c *session.Client, act *activity, date time.Time, minutes int, comment string) {
	year, weekNo := date.ISOWeek()
	raw, err := fetchWeek(c, year, weekNo)
	if err != nil {
		fail(err)
	}
	var w week
	if err := json.Unmarshal(raw, &w); err != nil {
		fail(fmt.Errorf("parse week: %w", err))
	}

	payload := map[string]any{
		"id":              nil,
		"type":            *act.HolidayLeaveType,
		"employeePartyId": fmt.Sprintf("%d", w.Data.EmployeeID),
		"activityId":      fmt.Sprintf("%d", act.ID),
		"fromDate":        date.Format("2006-01-02"),
		"toDate":          date.Format("2006-01-02"),
		"comment":         comment,
		"daysOrHours":     0,
		"minutes":         minutes,
		"days":            1,
	}

	if _, err := c.Do("POST", "/api/holidayandleave/dialog/holidayleavedialog/submitforapproval", payload); err != nil {
		fail(err)
	}

	display := fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
	fmt.Println(green.Render(fmt.Sprintf("✓ %s %s on %s submitted for approval", display, act.Name, date.Format("Mon 2006-01-02"))))
}

// submitInternalEntry calls saveEntry for type=250 internal activities (helligdag, kurs, etc).
// Discovers customerId/projectId from an existing line w/ same activity in any prior week.
func submitInternalEntry(c *session.Client, act *activity, date time.Time, minutes int, comment string, acts []activity) {
	// Find an existing line in current/prior week with same activityId to grab project/customer/dept.
	cust, proj, dept := lookupInternalContext(c, act.ID, date)
	if cust == 0 || proj == 0 || dept == 0 {
		fail(fmt.Errorf("could not auto-discover customer/project for activity %s — log one in the UI first, then `timef leave` will copy the line", act.Code))
	}

	var tsID any
	if act.TimeSpecificationID != nil {
		tsID = *act.TimeSpecificationID
	}

	payload := map[string]any{
		"id":                    "",
		"customerId":            fmt.Sprintf("%d", cust),
		"activityId":            fmt.Sprintf("%d", act.ID),
		"projectId":             fmt.Sprintf("%d", proj),
		"departmentId":          fmt.Sprintf("%d", dept),
		"timeSpecificationId":   tsID,
		"date":                  date.Format("2006-01-02"),
		"minutes":               minutes,
		"externalComment":       comment,
		"internalComment":       "",
		"isTimeOff":             false,
		"assignmentAgreementId": "",
		"qualityDeliveryAreaId": "",
	}

	if _, err := c.Do("PUT", "/api/timetracking/timesheet/timesheet/saveEntry", payload); err != nil {
		fail(err)
	}

	display := fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
	fmt.Println(green.Render(fmt.Sprintf("✓ %s %s on %s logged", display, act.Name, date.Format("Mon 2006-01-02"))))
}

// lookupInternalContext walks recent weeks looking for an existing line w/ the given activityId.
// Returns customerId, projectId, departmentId of first match.
func lookupInternalContext(c *session.Client, activityID int64, around time.Time) (int64, int64, int64) {
	for offset := 0; offset < 52; offset++ {
		t := around.AddDate(0, 0, -7*offset)
		year, weekNo := t.ISOWeek()
		raw, err := fetchWeek(c, year, weekNo)
		if err != nil {
			continue
		}

		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var resp map[string]any
		if err := dec.Decode(&resp); err != nil {
			continue
		}
		data, _ := resp["data"].(map[string]any)
		if data == nil {
			continue
		}
		lines, _ := data["lines"].([]any)
		for _, l := range lines {
			ln, _ := l.(map[string]any)
			actIDRaw := ln["activityId"]
			if actIDRaw == nil {
				continue
			}
			actID, err := strconv.ParseInt(strOrEmpty(actIDRaw), 10, 64)
			if err != nil || actID != activityID {
				continue
			}
			cust, _ := strconv.ParseInt(strOrEmpty(ln["customerId"]), 10, 64)
			proj, _ := strconv.ParseInt(strOrEmpty(ln["projectId"]), 10, 64)
			dept, _ := strconv.ParseInt(strOrEmpty(ln["departmentId"]), 10, 64)
			return cust, proj, dept
		}
	}
	return 0, 0, 0
}

func projectDisplayName(p project) string {
	if p.FullName != "" {
		return p.FullName
	}
	if p.SubprojectName != "" {
		return p.Name + " / " + p.SubprojectName
	}
	return p.Name
}

func cmdClear(args []string) {
	if len(args) < 2 {
		fail(fmt.Errorf("usage: timef clear <subprojectId> <date>"))
	}
	// Pass through to set with minutes=0.
	newArgs := append([]string{args[0], args[1], "0"}, args[2:]...)
	cmdSet(newArgs)
}

func fail(err error) {
	fmt.Println(red.Render(fmt.Sprintf("✗ %v", err)))
	os.Exit(1)
}
