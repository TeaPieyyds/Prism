package db

import (
	"encoding/json"
	"fmt"
	"time"
)

type Survey struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	RewardNuts    int    `json:"reward_nuts"`
	IsActive      bool   `json:"is_active"`
	HasAnswers    bool   `json:"has_answers"`
	CreatedAt     string `json:"created_at"`
	QuestionCount int    `json:"question_count,omitempty"`
	AnswerCount   int    `json:"answer_count,omitempty"`
}

type SurveyQuestion struct {
	ID           int64  `json:"id"`
	SurveyID     int64  `json:"survey_id"`
	QuestionText string `json:"question_text"`
	QuestionType string `json:"question_type"`
	SortOrder    int    `json:"sort_order"`
	Options      string `json:"options"`
	CreatedAt    string `json:"created_at"`
}

type SurveyAnswer struct {
	ID         int64  `json:"id"`
	SurveyID   int64  `json:"survey_id"`
	QuestionID int64  `json:"question_id"`
	UserID     int64  `json:"user_id"`
	AnswerText string `json:"answer_text"`
	CreatedAt  string `json:"created_at"`
}

type SurveyStats struct {
	Survey       *Survey              `json:"survey"`
	UserStatuses []UserAnswerStatus   `json:"user_statuses"`
	ChoiceStats  []ChoiceQuestionStat `json:"choice_stats"`
}

type UserAnswerStatus struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Answered bool   `json:"answered"`
}

type ChoiceQuestionStat struct {
	QuestionID   int64        `json:"question_id"`
	QuestionText string       `json:"question_text"`
	Options      []OptionStat `json:"options"`
}

type OptionStat struct {
	OptionText string  `json:"option_text"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

func CreateSurvey(title string, rewardNuts int, questions []SurveyQuestion) (*Survey, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO surveys (title, reward_nuts, is_active, has_answers, created_at) VALUES (?, ?, 0, 0, ?)`,
		title, rewardNuts, now)
	if err != nil {
		return nil, fmt.Errorf("创建问卷失败: %w", err)
	}
	surveyID, _ := res.LastInsertId()

	for i, q := range questions {
		if q.SortOrder == 0 {
			q.SortOrder = i
		}
		opts := q.Options
		if opts == "" {
			opts = "[]"
		}
		_, err := tx.Exec(`INSERT INTO survey_questions (survey_id, question_text, question_type, sort_order, options, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			surveyID, q.QuestionText, q.QuestionType, q.SortOrder, opts, now)
		if err != nil {
			return nil, fmt.Errorf("添加问题失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Survey{ID: surveyID, Title: title, RewardNuts: rewardNuts, CreatedAt: now}, nil
}

func GetSurvey(id int64) (*Survey, error) {
	var s Survey
	var active, hasAns int
	err := DB.QueryRow(`SELECT id, title, reward_nuts, is_active, has_answers, created_at FROM surveys WHERE id = ?`, id).
		Scan(&s.ID, &s.Title, &s.RewardNuts, &active, &hasAns, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.IsActive = active != 0
	s.HasAnswers = hasAns != 0
	return &s, nil
}

func GetSurveyQuestions(surveyID int64) ([]SurveyQuestion, error) {
	rows, err := DB.Query(`SELECT id, survey_id, question_text, question_type, sort_order, options, created_at FROM survey_questions WHERE survey_id = ? ORDER BY sort_order`, surveyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var qs []SurveyQuestion
	for rows.Next() {
		var q SurveyQuestion
		if err := rows.Scan(&q.ID, &q.SurveyID, &q.QuestionText, &q.QuestionType, &q.SortOrder, &q.Options, &q.CreatedAt); err != nil {
			return nil, err
		}
		qs = append(qs, q)
	}
	return qs, nil
}

func ListSurveys() ([]Survey, error) {
	rows, err := DB.Query(`SELECT s.id, s.title, s.reward_nuts, s.is_active, s.has_answers, s.created_at,
		(SELECT COUNT(*) FROM survey_questions q WHERE q.survey_id = s.id) as question_count,
		(SELECT COUNT(DISTINCT user_id) FROM survey_answers a WHERE a.survey_id = s.id) as answer_count
		FROM surveys s ORDER BY s.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Survey
	for rows.Next() {
		var s Survey
		var active, hasAns int
		if err := rows.Scan(&s.ID, &s.Title, &s.RewardNuts, &active, &hasAns, &s.CreatedAt, &s.QuestionCount, &s.AnswerCount); err != nil {
			return nil, err
		}
		s.IsActive = active != 0
		s.HasAnswers = hasAns != 0
		list = append(list, s)
	}
	return list, nil
}

func UpdateSurvey(id int64, title string, rewardNuts int) error {
	_, err := DB.Exec(`UPDATE surveys SET title = ?, reward_nuts = ? WHERE id = ?`, title, rewardNuts, id)
	return err
}

func UpdateSurveyQuestions(surveyID int64, questions []SurveyQuestion) error {
	oldRows, err := DB.Query(`SELECT id FROM survey_questions WHERE survey_id = ? ORDER BY sort_order`, surveyID)
	if err != nil {
		return err
	}
	defer oldRows.Close()
	var oldIDs []int64
	for oldRows.Next() {
		var id int64
		oldRows.Scan(&id)
		oldIDs = append(oldIDs, id)
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	for i, q := range questions {
		opts := q.Options
		if opts == "" {
			opts = "[]"
		}
		if i < len(oldIDs) {
			_, err := tx.Exec(`UPDATE survey_questions SET question_text=?, question_type=?, sort_order=?, options=? WHERE id=?`,
				q.QuestionText, q.QuestionType, i, opts, oldIDs[i])
			if err != nil {
				return fmt.Errorf("更新问题失败: %w", err)
			}
		} else {
			_, err := tx.Exec(`INSERT INTO survey_questions (survey_id, question_text, question_type, sort_order, options, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				surveyID, q.QuestionText, q.QuestionType, i, opts, now)
			if err != nil {
				return fmt.Errorf("添加问题失败: %w", err)
			}
		}
	}

	for _, id := range oldIDs[len(questions):] {
		tx.Exec(`DELETE FROM survey_questions WHERE id = ?`, id)
	}

	return tx.Commit()
}

func ToggleSurveyActive(id int64) error {
	_, err := DB.Exec(`UPDATE surveys SET is_active = CASE WHEN is_active = 0 THEN 1 ELSE 0 END WHERE id = ?`, id)
	return err
}

func DeleteSurvey(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM survey_answers WHERE survey_id = ?`, id)
	tx.Exec(`DELETE FROM survey_questions WHERE survey_id = ?`, id)
	tx.Exec(`DELETE FROM surveys WHERE id = ?`, id)
	return tx.Commit()
}

func GetActiveSurveys(userID int64) ([]Survey, error) {
	rows, err := DB.Query(`SELECT s.id, s.title, s.reward_nuts, s.is_active, s.has_answers, s.created_at
		FROM surveys s WHERE s.is_active = 1
		AND s.id NOT IN (SELECT DISTINCT survey_id FROM survey_answers WHERE user_id = ?)
		ORDER BY s.id`, userID)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	var list []Survey
	for rows.Next() {
		var s Survey
		var active, hasAns int
		if err := rows.Scan(&s.ID, &s.Title, &s.RewardNuts, &active, &hasAns, &s.CreatedAt); err != nil {
			continue
		}
		s.IsActive = active != 0
		s.HasAnswers = hasAns != 0
		list = append(list, s)
	}
	return list, nil
}

func HasUserAnsweredSurvey(surveyID, userID int64) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM survey_answers WHERE survey_id = ? AND user_id = ?`, surveyID, userID).Scan(&count)
	return count > 0, err
}

func SubmitSurveyAnswers(surveyID, userID int64, answers []SurveyAnswer) (int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, a := range answers {
		_, err := tx.Exec(`INSERT INTO survey_answers (survey_id, question_id, user_id, answer_text, created_at) VALUES (?, ?, ?, ?, ?)`,
			surveyID, a.QuestionID, userID, a.AnswerText, now)
		if err != nil {
			return 0, fmt.Errorf("保存答案失败: %w", err)
		}
	}

	tx.Exec(`UPDATE surveys SET has_answers = 1 WHERE id = ?`, surveyID)
	tx.Commit()

	var reward int
	DB.QueryRow(`SELECT reward_nuts FROM surveys WHERE id = ?`, surveyID).Scan(&reward)
	AddNuts(userID, reward, "survey_reward", nil, nil)
	return reward, nil
}

func GetSurveyStats(surveyID int64) (*SurveyStats, error) {
	s, err := GetSurvey(surveyID)
	if err != nil {
		return nil, err
	}

	rows, err := DB.Query(`SELECT u.id, u.username FROM users u ORDER BY (SELECT MAX(a.created_at) FROM survey_answers a WHERE a.survey_id = ? AND a.user_id = u.id) DESC NULLS LAST, u.id`, surveyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userStatuses []UserAnswerStatus
	for rows.Next() {
		var us UserAnswerStatus
		if err := rows.Scan(&us.UserID, &us.Username); err != nil {
			continue
		}
		answered, _ := HasUserAnsweredSurvey(surveyID, us.UserID)
		us.Answered = answered
		userStatuses = append(userStatuses, us)
	}

	questions, _ := GetSurveyQuestions(surveyID)
	var choiceStats []ChoiceQuestionStat
	var totalAnswerCount int
	DB.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM survey_answers WHERE survey_id = ?`, surveyID).Scan(&totalAnswerCount)

	for _, q := range questions {
		if q.QuestionType != "choice" && q.QuestionType != "multi_choice" {
			continue
		}
		isMulti := q.QuestionType == "multi_choice"
		var opts []string
		if err := json.Unmarshal([]byte(q.Options), &opts); err != nil {
			continue
		}
		var optionStats []OptionStat
		for _, opt := range opts {
			var count int
			if isMulti {
				DB.QueryRow(`SELECT COUNT(*) FROM survey_answers WHERE survey_id = ? AND question_id = ? AND (answer_text = ? OR answer_text LIKE ? OR answer_text LIKE ? OR answer_text LIKE ?)`,
					surveyID, q.ID, opt, opt+",%", "%,"+opt+",%", "%,"+opt).Scan(&count)
			} else {
				DB.QueryRow(`SELECT COUNT(*) FROM survey_answers WHERE survey_id = ? AND question_id = ? AND answer_text = ?`,
					surveyID, q.ID, opt).Scan(&count)
			}
			pct := float64(0)
			if totalAnswerCount > 0 {
				pct = float64(count) / float64(totalAnswerCount) * 100
			}
			optionStats = append(optionStats, OptionStat{OptionText: opt, Count: count, Percentage: pct})
		}
		choiceStats = append(choiceStats, ChoiceQuestionStat{
			QuestionID: q.ID, QuestionText: q.QuestionText, Options: optionStats,
		})
	}

	return &SurveyStats{Survey: s, UserStatuses: userStatuses, ChoiceStats: choiceStats}, nil
}

func ClearSurveyAnswers(surveyID int64, userID *int64) (int64, error) {
	if userID != nil {
		r, err := DB.Exec(`DELETE FROM survey_answers WHERE survey_id = ? AND user_id = ?`, surveyID, *userID)
		if err != nil { return 0, err }
		n, _ := r.RowsAffected()
		return n, nil
	}
	r, err := DB.Exec(`DELETE FROM survey_answers WHERE survey_id = ?`, surveyID)
	if err != nil { return 0, err }
	n, _ := r.RowsAffected()
	return n, nil
}

func GetUserSurveyAnswers(surveyID, userID int64) ([]SurveyAnswer, error) {
	rows, err := DB.Query(`SELECT id, survey_id, question_id, user_id, answer_text, created_at FROM survey_answers WHERE survey_id = ? AND user_id = ? ORDER BY id`, surveyID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SurveyAnswer
	for rows.Next() {
		var a SurveyAnswer
		if err := rows.Scan(&a.ID, &a.SurveyID, &a.QuestionID, &a.UserID, &a.AnswerText, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}
