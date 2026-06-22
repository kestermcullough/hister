// SPDX-FileContributor: Adam Tauber <asciimoo@gmail.com>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"github.com/rs/zerolog/log"
)

type migration struct {
	pre  func() error
	post func() error
}

var migrations = []migration{
	{
		post: func() error {
			return DB.Model(&HistoryLink{}).Where("pinned = ?", false).Update("pinned", true).Error
		},
	},
}

func migrationVersion() (int64, bool) {
	var dbVer int64
	if err := DB.Model(&Database{}).Select("version").First(&dbVer).Error; err != nil {
		return 0, false
	}
	return dbVer, true
}

func migratePre(dbVer int64) error {
	for i := dbVer; i < int64(len(migrations)); i++ {
		if migrations[i].pre == nil {
			continue
		}
		log.Info().Msgf("Running pre-migration for DB version %d", i+1)
		if err := migrations[i].pre(); err != nil {
			return err
		}
	}
	return nil
}

func migratePost(dbVer int64) error {
	migCount := 0
	for i := dbVer; i < int64(len(migrations)); i++ {
		if migrations[i].post != nil {
			log.Info().Msgf("Running post-migration for DB version %d", i+1)
			if err := migrations[i].post(); err != nil {
				return err
			}
		}
		if err := DB.Model(&Database{}).Where("id = 1").Update("version", i+1).Error; err != nil {
			return err
		}
		migCount++
	}
	if migCount > 0 {
		log.Debug().Int("Migrations performed", migCount).Msg("DB migrations completed")
	}
	return nil
}
