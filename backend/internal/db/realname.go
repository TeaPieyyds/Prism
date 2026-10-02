package db

import (
	"time"
)

type RealnamePreset struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IDNumber  string `json:"id_number"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// GetRealnamePresets returns only enabled presets (for client use)
func GetRealnamePresets() ([]*RealnamePreset, error) {
	rows, err := DB.Query(`SELECT id, name, id_number, enabled, created_at FROM realname_presets WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []*RealnamePreset
	for rows.Next() {
		var p RealnamePreset
		if err := rows.Scan(&p.ID, &p.Name, &p.IDNumber, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		presets = append(presets, &p)
	}
	return presets, nil
}

// GetAllRealnamePresets returns all presets (for admin)
func GetAllRealnamePresets() ([]*RealnamePreset, error) {
	rows, err := DB.Query(`SELECT id, name, id_number, enabled, created_at FROM realname_presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []*RealnamePreset
	for rows.Next() {
		var p RealnamePreset
		if err := rows.Scan(&p.ID, &p.Name, &p.IDNumber, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		presets = append(presets, &p)
	}
	return presets, nil
}

func AddRealnamePreset(name, idNumber string, enabled bool) (*RealnamePreset, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := DB.Exec(`INSERT INTO realname_presets (name, id_number, enabled, created_at) VALUES (?, ?, ?, ?)`, name, idNumber, enabled, now)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &RealnamePreset{ID: id, Name: name, IDNumber: idNumber, Enabled: enabled, CreatedAt: now}, nil
}

func SetRealnamePresetEnabled(id int64, enabled bool) error {
	_, err := DB.Exec(`UPDATE realname_presets SET enabled = ? WHERE id = ?`, enabled, id)
	return err
}

func DeleteRealnamePreset(id int64) error {
	_, err := DB.Exec(`DELETE FROM realname_presets WHERE id = ?`, id)
	return err
}
