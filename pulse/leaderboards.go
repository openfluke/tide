package pulse

// RebuildLeaderboards ranks finished ok/gap cells for PDF export and reports.
func RebuildLeaderboards(completed []Result) (byScore, byMobile, byLearn, byLearnMobile []Result) {
	pool := make([]Result, 0, len(completed))
	for _, r := range completed {
		if r.Status == "ok" || r.Status == "gap" {
			pool = append(pool, r)
		}
	}
	return rankByScore(pool), rankByMobile(pool), rankByLearn(pool), rankByLearnMobile(pool)
}
