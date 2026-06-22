// SPDX-FileContributor: Adam Tauber <asciimoo@gmail.com>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"errors"
	"strings"
	"time"
)

type History struct {
	CommonFields
	UserID uint    `gorm:"uniqueIndex:useridqueryidx;default:0" json:"user_id"`
	Query  string  `gorm:"uniqueIndex:useridqueryidx" json:"query"`
	Links  []*Link `gorm:"many2many:history_links;" json:"urls"`
}

type Link struct {
	CommonFields
	URL   string `gorm:"unique" json:"url"`
	Title string `json:"title"`
}

type HistoryLink struct {
	CommonFields
	HistoryID uint     `gorm:"uniqueIndex:historylinkuidx"`
	History   *History `json:"history"`
	LinkID    uint     `gorm:"uniqueIndex:historylinkuidx"`
	Link      *Link    `json:"link"`
	Count     uint     `gorm:"type:uint" json:"count"`
	Pinned    bool     `gorm:"default:false" json:"pinned"`
}

type URLCount struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Text   string `gorm:"-" json:"text,omitempty"`
	Count  uint   `json:"count"`
	Pinned bool   `json:"pinned"`
}

type HistoryItem struct {
	ID        uint      `json:"id"`
	Query     string    `json:"query"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func GetOrCreateLink(u, title string) *Link {
	var ret *Link
	if err := DB.Model(&Link{}).Where("url = ?", u).First(&ret).Error; err != nil {
		ret = &Link{
			URL:   u,
			Title: title,
		}
		if err := DB.Create(ret).Error; err != nil {
			return nil
		}
	} else if ret.Title != title && title != "" {
		ret.Title = title
		DB.Save(ret)
	}
	return ret
}

func GetOrCreateHistory(userID uint, q string) *History {
	var ret *History
	if err := DB.Model(&History{}).Where("user_id = ? AND query = ?", userID, q).First(&ret).Error; err != nil {
		ret = &History{
			UserID: userID,
			Query:  q,
		}
		if err := DB.Create(ret).Error; err != nil {
			return nil
		}
	}
	return ret
}

func DeleteHistoryURL(userID uint, url string) error {
	subQ := DB.Table("history_links").
		Select("history_links.id").
		Joins("JOIN histories ON history_links.history_id = histories.id").
		Joins("JOIN links ON history_links.link_id = links.id").
		Where("histories.user_id = ? AND links.url = ?", userID, url)
	return DB.Delete(&HistoryLink{}, "id in (?)", subQ).Error
}

func DeleteHistoryItem(userID uint, query, url string) error {
	return DB.Delete(
		&HistoryLink{},
		"id in (?)",
		DB.Table("history_links").
			Select("history_links.id").
			Joins("JOIN histories ON history_links.history_id = histories.id").
			Joins("JOIN links ON history_links.link_id = links.id").
			Where("histories.user_id = ? AND histories.query = ? AND links.url = ?", userID, query, url),
	).Error
}

func SetHistoryPinned(userID uint, query, url, title string, pinned bool) error {
	if query == "" || url == "" {
		return errors.New("missing data")
	}
	if !pinned {
		return DB.Model(&HistoryLink{}).
			Where("id in (?)",
				DB.Table("history_links").
					Select("history_links.id").
					Joins("JOIN histories ON history_links.history_id = histories.id").
					Joins("JOIN links ON history_links.link_id = links.id").
					Where("histories.user_id = ? AND histories.query = ? AND links.url = ?", userID, query, url),
			).UpdateColumn("pinned", false).Error
	}
	l := GetOrCreateLink(url, title)
	h := GetOrCreateHistory(userID, query)
	if l == nil || h == nil {
		return errors.New("failed to get link or query")
	}
	var hu *HistoryLink
	if err := DB.Model(&HistoryLink{}).Where("history_id = ? AND link_id = ?", h.ID, l.ID).First(&hu).Error; err != nil {
		hu = &HistoryLink{
			HistoryID: h.ID,
			LinkID:    l.ID,
			Count:     1,
			Pinned:    true,
		}
		return DB.Create(hu).Error
	}
	hu.Pinned = true
	return DB.Save(hu).Error
}

func UpdateHistory(userID uint, query, url, title string) error {
	if query == "" || url == "" || title == "" {
		return errors.New("missing data")
	}
	l := GetOrCreateLink(url, title)
	h := GetOrCreateHistory(userID, query)
	if l == nil || h == nil {
		return errors.New("failed to get link or query")
	}
	var hu *HistoryLink
	if err := DB.Model(&HistoryLink{}).Where("history_id = ? AND link_id = ?", h.ID, l.ID).First(&hu).Error; err != nil {
		hu = &HistoryLink{
			HistoryID: h.ID,
			LinkID:    l.ID,
			Count:     1,
		}
		return DB.Create(hu).Error
	}
	hu.Count += 1
	return DB.Save(hu).Error
}

func GetURLsByQuery(userID uint, q string) ([]*URLCount, error) {
	var us []*URLCount
	err := DB.Select("links.url as url, links.title as title, history_links.count as count, history_links.pinned as pinned").
		Table("history_links").
		Joins("JOIN links ON history_links.link_id = links.id").
		Joins("JOIN histories ON history_links.history_id = histories.id").
		Where("histories.user_id = ? AND histories.query = ?", userID, q).
		Order("history_links.pinned DESC, history_links.count DESC, history_links.updated_at DESC").
		Limit(20).Find(&us).Error
	return us, err
}

func GetLatestHistoryItems(userID uint, limit int, lastID uint) ([]*HistoryItem, error) {
	var hs []*HistoryItem
	q := DB.Select("history_links.id as id, links.url as url, links.title as title, histories.query as query, history_links.updated_at as updated_at").
		Table("history_links").
		Joins("JOIN links ON history_links.link_id = links.id").
		Joins("JOIN histories ON history_links.history_id = histories.id").
		Where("histories.user_id = ?", userID).
		Order("history_links.updated_at DESC")
	if lastID > 0 {
		q = q.Where("history_links.id < ?", lastID)
	}
	err := q.Limit(limit).Find(&hs).Error
	return hs, err
}

func GetQuerySuggestion(userID uint, q string) string {
	var r string
	DB.Select("histories.query as query").
		Table("history_links").
		Joins("JOIN histories ON history_links.history_id = histories.id").
		Where("histories.user_id = ? AND LOWER(histories.query) LIKE ?", userID, strings.ToLower(q+"%")).
		Order("history_links.count DESC").
		Limit(1).Find(&r)
	return r
}
