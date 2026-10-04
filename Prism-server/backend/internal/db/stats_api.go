package db

import (
	"fmt"
	"strings"
	"time"
)

// ── Test account filtering ──

// testUsernamePatterns lists username substrings that mark an admin-created test
// account. These accounts pollute real usage stats, so admin-level aggregates
// exclude them by default. EXTEND THIS LIST as needed.
var testUsernamePatterns = []string{
	"测试", "测试号", "test号", "test_", "test-",
	"猫测试", "猫test", "猫测试号", "猫test号",
	"猫", // cat test accounts like 猫白, 猫七街
	"测试服", "内测", "公测", "体验号",
}

// excludeTestUsersSQL builds a WHERE fragment that excludes users whose username
// matches any test pattern. Returns "" when no test users should be excluded.
// combined with the CALLER's existing WHERE (must be non-empty) via AND.
func excludeTestUsersSQL(exclude bool) (string, []any) {
	if !exclude {
		return "", nil
	}
	var conds []string
	var args []any
	for _, p := range testUsernamePatterns {
		conds = append(conds, "u.username NOT LIKE ?")
		args = append(args, "%"+p+"%")
	}
	return "(" + strings.Join(conds, " AND ") + ")", args
}

// isTestUsername reports whether a username matches any test pattern.
func isTestUsername(name string) bool {
	for _, p := range testUsernamePatterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// ── Types ──

type EndpointStat struct {
	Time   string `json:"time"`
	Total  int    `json:"total"`
	Errors int    `json:"errors"`
}

type EndpointBreakdown struct {
	Endpoint string         `json:"endpoint"`
	Data     []EndpointStat `json:"data"`
}

type UserStatsSummary struct {
	UserID      int64   `json:"user_id"`
	Username    string  `json:"username"`
	TotalCalls  int     `json:"total_calls"`
	Success     int     `json:"success"`
	Errors      int     `json:"errors"`
	SuccessRate float64 `json:"success_rate"`
	JoinCount   int     `json:"join_count"`
	NutsUsed    int     `json:"nuts_used"`
}

type ServerJoinStat struct {
	ServerCode string `json:"server_code"`
	Count      int    `json:"count"`
}

type NutsTimelinePoint struct {
	Time    string `json:"time"`
	Balance int    `json:"balance"`
}

type AdminOverview struct {
	TotalCalls   int     `json:"total_calls"`
	SuccessRate  float64 `json:"success_rate"`
	ActiveUsers  int     `json:"active_users"`
	TotalUsers   int     `json:"total_users"`
	TodayRegs    int     `json:"today_regs"`
	TotalJoins   int     `json:"total_joins"`
}

type RegistrationStat struct {
	Time  string `json:"time"`
	Count int    `json:"count"`
}

// ── Record API call ──

func RecordAPICall(userID *int64, endpoint, method string, success bool, statusCode int, durationMs int64, ip, detail string, apiTokenID *int64, apiTokenName string) {
	s := 0
	if success {
		s = 1
	}
	if len(detail) > 200 {
		detail = detail[:200]
	}
	DB.Exec(
		`INSERT INTO api_call_logs (user_id, endpoint, method, success, status_code, duration_ms, ip, detail, api_token_id, api_token_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, endpoint, method, s, statusCode, durationMs, ip, detail, apiTokenID, apiTokenName, time.Now().UTC().Format(time.RFC3339),
	)
}

// ── User stats ──

func GetUserStatsSummary(userID int64, hours int) (UserStatsSummary, error) {
	s := UserStatsSummary{UserID: userID}
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	row := DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0)
		FROM api_call_logs WHERE user_id = ? AND created_at >= ?`, userID, cutoff)
	if err := row.Scan(&s.TotalCalls, &s.Success, &s.Errors); err != nil {
		return s, err
	}
	if s.TotalCalls > 0 {
		s.SuccessRate = float64(s.Success) / float64(s.TotalCalls) * 100
	}

	DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'join_server' AND created_at >= ?`, userID, cutoff).Scan(&s.JoinCount)

	DB.QueryRow(`SELECT COALESCE(SUM(ABS(amount)),0) FROM nuts_transactions WHERE user_id = ? AND amount < 0 AND created_at >= ?`, userID, cutoff).Scan(&s.NutsUsed)

	DB.QueryRow(`SELECT COALESCE(u.username, '') FROM users u WHERE u.id = ?`, userID).Scan(&s.Username)
	return s, nil
}

func GetUserEndpointStats(userID int64, granularity string, hours int) ([]EndpointBreakdown, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	timeFmt := timeFormat(granularity)

	rows, err := DB.Query(`
		SELECT endpoint, strftime(?, created_at) as bucket, COUNT(*) as total, COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0) as errors
		FROM api_call_logs
		WHERE user_id = ? AND created_at >= ?
		GROUP BY endpoint, bucket
		ORDER BY endpoint, bucket`, timeFmt, userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		endpoint, bucket string
		total, errors    int
	}
	raw := []row{}
	for rows.Next() {
		var r row
		rows.Scan(&r.endpoint, &r.bucket, &r.total, &r.errors)
		raw = append(raw, r)
	}

	epMap := map[string][]EndpointStat{}
	epOrder := []string{}
	for _, r := range raw {
		if _, ok := epMap[r.endpoint]; !ok {
			epOrder = append(epOrder, r.endpoint)
		}
		epMap[r.endpoint] = append(epMap[r.endpoint], EndpointStat{Time: r.bucket, Total: r.total, Errors: r.errors})
	}

	result := []EndpointBreakdown{}
	for _, ep := range epOrder {
		result = append(result, EndpointBreakdown{Endpoint: ep, Data: fillEndpointBuckets(epMap[ep], granularity, hours)})
	}
	if result == nil {
		result = []EndpointBreakdown{}
	}
	return result, nil
}

func GetUserServerJoinStats(userID int64, hours int) ([]ServerJoinStat, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	rows, err := DB.Query(`
		SELECT target, COUNT(*) as cnt FROM audit_logs
		WHERE user_id = ? AND action = 'join_server' AND created_at >= ?
		GROUP BY target ORDER BY cnt DESC LIMIT 20`, userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := []ServerJoinStat{}
	for rows.Next() {
		var s ServerJoinStat
		rows.Scan(&s.ServerCode, &s.Count)
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []ServerJoinStat{}
	}
	return stats, nil
}

func GetUserNutsTimeline(userID int64, hours int) ([]NutsTimelinePoint, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	rows, err := DB.Query(`
		SELECT created_at, balance_after FROM nuts_transactions
		WHERE user_id = ? AND created_at >= ?
		ORDER BY created_at`, userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []NutsTimelinePoint{}
	for rows.Next() {
		var p NutsTimelinePoint
		rows.Scan(&p.Time, &p.Balance)
		points = append(points, p)
	}
	// Prepend current balance at cutoff if no transactions
	if len(points) == 0 {
		var bal int
		DB.QueryRow(`SELECT COALESCE(nuts_balance,0) FROM users WHERE id = ?`, userID).Scan(&bal)
		points = append(points, NutsTimelinePoint{Time: cutoff, Balance: bal})
	}
	if points == nil {
		points = []NutsTimelinePoint{}
	}
	return points, nil
}

func GetUserActiveEndpoints(userID int64) ([]string, error) {
	rows, err := DB.Query(`SELECT DISTINCT endpoint FROM api_call_logs WHERE user_id = ? ORDER BY endpoint`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	eps := []string{}
	for rows.Next() {
		var e string
		rows.Scan(&e)
		eps = append(eps, e)
	}
	return eps, nil
}

// ── Admin stats ──

func GetAdminOverview(hours int, excludeTest bool) (AdminOverview, error) {
	o := AdminOverview{}
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)

	if excludeTest {
		DB.QueryRow(`SELECT COUNT(*) FROM api_call_logs a INNER JOIN users u ON u.id = a.user_id WHERE a.created_at >= ? AND `+exclCond, append([]any{cutoff}, exclArgs...)...).Scan(&o.TotalCalls)
	} else {
		DB.QueryRow(`SELECT COUNT(*) FROM api_call_logs WHERE created_at >= ?`, cutoff).Scan(&o.TotalCalls)
	}

	var success, errors int
	if excludeTest {
		DB.QueryRow(`SELECT COALESCE(SUM(CASE WHEN a.success=1 THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN a.success=0 THEN 1 ELSE 0 END),0) FROM api_call_logs a INNER JOIN users u ON u.id = a.user_id WHERE a.created_at >= ? AND `+exclCond, append([]any{cutoff}, exclArgs...)...).Scan(&success, &errors)
	} else {
		DB.QueryRow(`SELECT COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0) FROM api_call_logs WHERE created_at >= ?`, cutoff).Scan(&success, &errors)
	}
	if success+errors > 0 {
		o.SuccessRate = float64(success) / float64(success+errors) * 100
	}

	if excludeTest {
		DB.QueryRow(`SELECT COUNT(DISTINCT a.user_id) FROM api_call_logs a INNER JOIN users u ON u.id = a.user_id WHERE a.user_id IS NOT NULL AND a.created_at >= ? AND `+exclCond, append([]any{cutoff}, exclArgs...)...).Scan(&o.ActiveUsers)
	} else {
		DB.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM api_call_logs WHERE user_id IS NOT NULL AND created_at >= ?`, cutoff).Scan(&o.ActiveUsers)
	}

	if excludeTest {
		DB.QueryRow(`SELECT COUNT(*) FROM users u WHERE `+exclCond, exclArgs...).Scan(&o.TotalUsers)
	} else {
		DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&o.TotalUsers)
	}

	if excludeTest {
		DB.QueryRow(`SELECT COUNT(*) FROM users u WHERE created_at >= ? AND `+exclCond, append([]any{cutoff}, exclArgs...)...).Scan(&o.TodayRegs)
	} else {
		DB.QueryRow(`SELECT COUNT(*) FROM users WHERE created_at >= ?`, time.Now().UTC().Format("2006-01-02")+"T00:00:00Z").Scan(&o.TodayRegs)
	}

	if excludeTest {
		DB.QueryRow(`SELECT COUNT(*) FROM audit_logs al INNER JOIN users u ON u.id = al.user_id WHERE al.action = 'join_server' AND al.created_at >= ? AND `+exclCond, append([]any{cutoff}, exclArgs...)...).Scan(&o.TotalJoins)
	} else {
		DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'join_server' AND created_at >= ?`, cutoff).Scan(&o.TotalJoins)
	}
	return o, nil
}

func GetAdminEndpointStats(granularity string, hours int, excludeTest bool) ([]EndpointBreakdown, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	timeFmt := timeFormat(granularity)

	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT a.endpoint, strftime(?, a.created_at) as bucket, COUNT(*) as total, COALESCE(SUM(CASE WHEN a.success=0 THEN 1 ELSE 0 END),0) as errors
		FROM api_call_logs a
		INNER JOIN users u ON u.id = a.user_id
		WHERE a.created_at >= ?`
	if exclCond != "" {
		query += " AND " + exclCond
	}
	query += `
		GROUP BY a.endpoint, bucket
		ORDER BY a.endpoint, bucket`
	args := []any{timeFmt, cutoff}
	args = append(args, exclArgs...)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		endpoint, bucket string
		total, errors    int
	}
	raw := []row{}
	for rows.Next() {
		var r row
		rows.Scan(&r.endpoint, &r.bucket, &r.total, &r.errors)
		raw = append(raw, r)
	}

	epMap := map[string][]EndpointStat{}
	epOrder := []string{}
	for _, r := range raw {
		if _, ok := epMap[r.endpoint]; !ok {
			epOrder = append(epOrder, r.endpoint)
		}
		epMap[r.endpoint] = append(epMap[r.endpoint], EndpointStat{Time: r.bucket, Total: r.total, Errors: r.errors})
	}

	result := []EndpointBreakdown{}
	for _, ep := range epOrder {
		result = append(result, EndpointBreakdown{Endpoint: ep, Data: fillEndpointBuckets(epMap[ep], granularity, hours)})
	}
	if result == nil {
		result = []EndpointBreakdown{}
	}
	return result, nil
}

func GetAdminPerUserStats(hours int, excludeTest bool) ([]UserStatsSummary, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT a.user_id, COALESCE(u.username, ''), COUNT(*) as total,
			COALESCE(SUM(CASE WHEN a.success=1 THEN 1 ELSE 0 END),0) as ok,
			COALESCE(SUM(CASE WHEN a.success=0 THEN 1 ELSE 0 END),0) as err,
			COALESCE((SELECT COUNT(*) FROM audit_logs al WHERE al.user_id = a.user_id AND al.action = 'join_server' AND al.created_at >= ?), 0) as join_cnt,
			COALESCE((SELECT SUM(ABS(amount)) FROM nuts_transactions nt WHERE nt.user_id = a.user_id AND nt.amount < 0 AND nt.created_at >= ?), 0) as nuts_used
		FROM api_call_logs a
		LEFT JOIN users u ON u.id = a.user_id
		WHERE a.user_id IS NOT NULL AND a.created_at >= ?`
	if exclCond != "" {
		query += " AND " + exclCond
	}
	query += `
		GROUP BY a.user_id
		ORDER BY total DESC`
	args := []any{cutoff, cutoff, cutoff}
	args = append(args, exclArgs...)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []UserStatsSummary{}
	for rows.Next() {
		var s UserStatsSummary
		rows.Scan(&s.UserID, &s.Username, &s.TotalCalls, &s.Success, &s.Errors, &s.JoinCount, &s.NutsUsed)
		if s.TotalCalls > 0 {
			s.SuccessRate = float64(s.Success) / float64(s.TotalCalls) * 100
		}
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []UserStatsSummary{}
	}
	return stats, nil
}

func GetAdminRegistrationStats(granularity string, hours int, excludeTest bool) ([]RegistrationStat, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
	timeFmt := timeFormat(granularity)

	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT strftime(?, created_at) as bucket, COUNT(*) as cnt
		FROM users WHERE created_at >= ?`
	if exclCond != "" {
		query += " AND " + exclCond
	}
	query += `
		GROUP BY bucket ORDER BY bucket`
	args := []any{timeFmt, cutoff}
	args = append(args, exclArgs...)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := []RegistrationStat{}
	for rows.Next() {
		var s RegistrationStat
		rows.Scan(&s.Time, &s.Count)
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []RegistrationStat{}
	}
	return stats, nil
}

// ── Helpers ──

// timeFormat returns the SQLite strftime format for a granularity.
// The 15min case uses a custom expression that floors minutes to quarter-hour.
func timeFormat(granularity string) string {
	switch granularity {
	case "minute":
		return "%Y-%m-%dT%H:%M:00"
	case "15min":
		return `strftime('%Y-%m-%dT%H:', created_at) || printf('%02d', (CAST(strftime('%M', created_at) AS INTEGER)/15)*15) || ':00'`
	case "day":
		return "%Y-%m-%d"
	case "week":
		return "%Y-W%W"
	case "month":
		return "%Y-%m"
	default:
		return "%Y-%m-%dT%H:00"
	}
}

// bucketStep returns the time.Duration between buckets for a granularity.
func bucketStep(granularity string) time.Duration {
	switch granularity {
	case "minute":
		return time.Minute
	case "15min":
		return 15 * time.Minute
	case "hour":
		return time.Hour
	case "day":
		return 24 * time.Hour
	case "week":
		return 7 * 24 * time.Hour
	case "month":
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}

// formatBucket renders a time.Time into the same string form the SQL strftime
// produces for a given granularity, so Go-generated buckets and SQL buckets match.
func formatBucket(t time.Time, granularity string) string {
	switch granularity {
	case "minute":
		return t.Format("2006-01-02T15:04:00")
	case "15min":
		floor := (t.Minute() / 15) * 15
		return t.Format("2006-01-02T15:") + fmt.Sprintf("%02d", floor) + ":00"
	case "day":
		return t.Format("2006-01-02")
	case "week":
		// ISO-ish week string matching SQLite %Y-W%W (week starts Monday, W%W 00-53)
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "month":
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02T15:00")
	}
}

// buildBucketList generates a contiguous, ascending list of bucket strings from
// cutoff to now, matching the SQL strftime output for the granularity.
func buildBucketList(cutoff time.Time, granularity string) []string {
	now := time.Now().UTC()
	step := bucketStep(granularity)
	var buckets []string
	seen := map[string]bool{}
	for t := cutoff; !t.After(now); t = t.Add(step) {
		b := formatBucket(t, granularity)
		if seen[b] {
			continue
		}
		seen[b] = true
		buckets = append(buckets, b)
	}
	if len(buckets) == 0 {
		buckets = append(buckets, formatBucket(now, granularity))
	}
	return buckets
}

// fillEndpointBuckets ensures every endpoint has a stat for every bucket in the
// range, zero-filling the gaps so multi-endpoint line charts align correctly.
// If the bucket count is too large (>1000) it returns raw data to avoid OOM issues.
func fillEndpointBuckets(raw []EndpointStat, granularity string, hours int) []EndpointStat {
	now := time.Now().UTC()
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	buckets := buildBucketList(cutoff, granularity)
	if len(buckets) > 1000 {
		return raw
	}

	byBucket := map[string]EndpointStat{}
	for _, s := range raw {
		byBucket[s.Time] = s
	}
	out := make([]EndpointStat, 0, len(buckets))
	for _, b := range buckets {
		if s, ok := byBucket[b]; ok {
			out = append(out, s)
		} else {
			out = append(out, EndpointStat{Time: b, Total: 0, Errors: 0})
		}
	}
	return out
}

type UserDetailStats struct {
	UserID      int64            `json:"user_id"`
	Username    string           `json:"username"`
	TotalCalls  int              `json:"total_calls"`
	Success     int              `json:"success"`
	Errors      int              `json:"errors"`
	SuccessRate float64          `json:"success_rate"`
	JoinCount   int              `json:"join_count"`
	Servers     []ServerJoinStat `json:"servers"`
	NutsBalance int              `json:"nuts_balance"`
	NutsUsed    int              `json:"nuts_used"`
	Endpoints   []EndpointBreakdown `json:"endpoints"`
}

func GetUserDetailStats(userID int64) (UserDetailStats, error) {
	s := UserDetailStats{UserID: userID}
	DB.QueryRow(`SELECT COALESCE(username,'') FROM users WHERE id = ?`, userID).Scan(&s.Username)

	DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0)
		FROM api_call_logs WHERE user_id = ?`, userID).Scan(&s.TotalCalls, &s.Success, &s.Errors)
	if s.TotalCalls > 0 {
		s.SuccessRate = float64(s.Success) / float64(s.TotalCalls) * 100
	}

	DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND action = 'join_server'`, userID).Scan(&s.JoinCount)
	DB.QueryRow(`SELECT COALESCE(nuts_balance,0) FROM users WHERE id = ?`, userID).Scan(&s.NutsBalance)
	DB.QueryRow(`SELECT COALESCE(SUM(ABS(amount)),0) FROM nuts_transactions WHERE user_id = ? AND amount < 0`, userID).Scan(&s.NutsUsed)

	s.Servers, _ = GetUserServerJoinStats(userID, 87600) // all time
	if s.Servers == nil { s.Servers = []ServerJoinStat{} }

	s.Endpoints, _ = GetUserEndpointStats(userID, "day", 720)
	if s.Endpoints == nil { s.Endpoints = []EndpointBreakdown{} }

	return s, nil
}

// ── Additional admin stats dimensions ──

type EndpointErrorStat struct {
	Endpoint string  `json:"endpoint"`
	Total    int     `json:"total"`
	Errors   int     `json:"errors"`
	ErrRate  float64 `json:"err_rate"`
}

type HourlyStat struct {
	Time  string `json:"time"`
	Count int    `json:"count"`
}

// GetAdminErrorStats ranks endpoints by error count/rate within the range.
func GetAdminErrorStats(hours int, excludeTest bool) ([]EndpointErrorStat, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT a.endpoint, COUNT(*) as total, COALESCE(SUM(CASE WHEN a.success=0 THEN 1 ELSE 0 END),0) as errors
		FROM api_call_logs a
		INNER JOIN users u ON u.id = a.user_id
		WHERE a.created_at >= ?`
	if exclCond != "" {
		query += " AND " + exclCond
	}
	query += `
		GROUP BY a.endpoint HAVING errors > 0
		ORDER BY errors DESC, total DESC LIMIT 30`
	args := []any{cutoff}
	args = append(args, exclArgs...)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := []EndpointErrorStat{}
	for rows.Next() {
		var s EndpointErrorStat
		if err := rows.Scan(&s.Endpoint, &s.Total, &s.Errors); err != nil {
			return nil, err
		}
		if s.Total > 0 {
			s.ErrRate = float64(s.Errors) / float64(s.Total) * 100
		}
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []EndpointErrorStat{}
	}
	return stats, nil
}

// GetAdminBusyHours returns the per-hour API call distribution across all time,
// used to spot peak hours.
func GetAdminBusyHours(excludeTest bool) ([]HourlyStat, error) {
	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT strftime('%H:00', a.created_at) as h, COUNT(*) as cnt
		FROM api_call_logs a
		INNER JOIN users u ON u.id = a.user_id`
	if exclCond != "" {
		query += " WHERE " + exclCond
	}
	query += ` GROUP BY h ORDER BY cnt DESC LIMIT 24`
	rows, err := DB.Query(query, exclArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := []HourlyStat{}
	for rows.Next() {
		var s HourlyStat
		if err := rows.Scan(&s.Time, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []HourlyStat{}
	}
	return stats, nil
}

// GetAdminMethodStats breaks down API calls by HTTP method.
func GetAdminMethodStats(excludeTest bool) ([]HourlyStat, error) {
	exclCond, exclArgs := excludeTestUsersSQL(excludeTest)
	query := `
		SELECT COALESCE(NULLIF(a.method,''), 'GET') as m, COUNT(*) as cnt
		FROM api_call_logs a
		INNER JOIN users u ON u.id = a.user_id`
	if exclCond != "" {
		query += " WHERE " + exclCond
	}
	query += ` GROUP BY m ORDER BY cnt DESC`
	rows, err := DB.Query(query, exclArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := []HourlyStat{}
	for rows.Next() {
		var s HourlyStat
		if err := rows.Scan(&s.Time, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []HourlyStat{}
	}
	return stats, nil
}

// ── Backfill ──

func BackfillFromAuditLogs() error {
	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM api_call_logs`).Scan(&count)
	if count > 0 {
		return nil
	}

	rows, err := DB.Query(`SELECT user_id, action, target, created_at FROM audit_logs WHERE action IN ('join_server','join_server_failed','register_verified','login') ORDER BY id`)
	if err != nil {
		return err
	}

	type rec struct {
		userID    int64
		endpoint  string
		method    string
		success   int
		action    string
		createdAt string
	}
	var recs []rec
	for rows.Next() {
		var userID int64
		var action, target, createdAt string
		if err := rows.Scan(&userID, &action, &target, &createdAt); err != nil {
			continue
		}
		_ = target
		var endpoint, method string
		success := true
		switch action {
		case "join_server":
			endpoint, method, success = "/api/phoenix/login", "POST", true
		case "join_server_failed":
			endpoint, method, success = "/api/phoenix/login", "POST", false
		case "register_verified":
			endpoint, method, success = "/api/auth/register", "POST", true
		case "login":
			endpoint, method, success = "/api/auth/login", "POST", true
		default:
			continue
		}
		s := 0
		if success {
			s = 1
		}
		recs = append(recs, rec{userID, endpoint, method, s, action, createdAt})
	}
	rows.Close()

	inserted := 0
	for _, r := range recs {
		_, err := DB.Exec(
			`INSERT OR IGNORE INTO api_call_logs (user_id, endpoint, method, success, status_code, duration_ms, ip, detail, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.userID, r.endpoint, r.method, r.success, 200, 0, "", r.action, r.createdAt)
		if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
			continue
		}
		inserted++
	}
	if inserted > 0 {
		fmt.Printf("[STATS] backfilled %d events from audit_logs\n", inserted)
	}
	return nil
}
