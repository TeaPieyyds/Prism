package db

import "time"

type SkinPreset struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ItemID     string `json:"item_id"`
	PreviewURL string `json:"preview_url"`
	CreatedAt  string `json:"created_at"`
}

func ListSkinPresets() ([]SkinPreset, error) {
	rows, err := DB.Query(`SELECT id, name, item_id, preview_url, created_at FROM skin_presets ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []SkinPreset
	for rows.Next() {
		var p SkinPreset
		if err := rows.Scan(&p.ID, &p.Name, &p.ItemID, &p.PreviewURL, &p.CreatedAt); err != nil {
			return nil, err
		}
		presets = append(presets, p)
	}
	return presets, nil
}

func AddSkinPreset(name, itemID, previewURL string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT OR IGNORE INTO skin_presets (name, item_id, preview_url, created_at) VALUES (?, ?, ?, ?)`,
		name, itemID, previewURL, now)
	return err
}

func DeleteSkinPreset(id int64) error {
	_, err := DB.Exec(`DELETE FROM skin_presets WHERE id = ?`, id)
	return err
}

func UpdateSkinPresetPreview(itemID, previewURL string) error {
	_, err := DB.Exec(`UPDATE skin_presets SET preview_url = ? WHERE item_id = ?`, previewURL, itemID)
	return err
}

func UpdateSkinPreset(id int64, name, itemID string) error {
	_, err := DB.Exec(`UPDATE skin_presets SET name = ?, item_id = ? WHERE id = ?`, name, itemID, id)
	return err
}
