// Package holiday computes Norwegian public holidays.
package holiday

import "time"

// Norway returns a map of date → holiday name for the given year.
// Dates use time.Date(y, m, d, 0, 0, 0, 0, time.UTC) as keys.
func Norway(year int) map[time.Time]string {
	out := map[time.Time]string{}
	add := func(t time.Time, name string) {
		out[time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)] = name
	}

	easter := easterSunday(year)
	add(easter.AddDate(0, 0, -3), "Skjærtorsdag")
	add(easter.AddDate(0, 0, -2), "Langfredag")
	add(easter, "1. påskedag")
	add(easter.AddDate(0, 0, 1), "2. påskedag")
	add(easter.AddDate(0, 0, 39), "Kristi himmelfartsdag")
	add(easter.AddDate(0, 0, 49), "1. pinsedag")
	add(easter.AddDate(0, 0, 50), "2. pinsedag")

	add(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), "Nyttårsdag")
	add(time.Date(year, 5, 1, 0, 0, 0, 0, time.UTC), "Arbeidernes dag")
	add(time.Date(year, 5, 17, 0, 0, 0, 0, time.UTC), "Grunnlovsdag")
	add(time.Date(year, 12, 25, 0, 0, 0, 0, time.UTC), "1. juledag")
	add(time.Date(year, 12, 26, 0, 0, 0, 0, time.UTC), "2. juledag")

	return out
}

// Lookup returns the holiday name for a date in Norway, or "" if not a holiday.
func Lookup(t time.Time) string {
	key := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return Norway(t.Year())[key]
}

// easterSunday computes Easter Sunday via the Anonymous Gregorian algorithm.
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
