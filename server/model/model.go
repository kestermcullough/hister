// SPDX-FileContributor: Adam Tauber <asciimoo@gmail.com>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/asciimoo/hister/config"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
)

// ErrDBType is returned when an unknown database type is encountered.
var ErrDBType = errors.New("unknown database type")

// DB is the global database instance.
var DB *gorm.DB

// Init initializes the database connection and runs migrations.
func Init(c *config.Config) error {
	dbCfg := &gorm.Config{}
	if c.App.DebugSQL {
		dbCfg.Logger = logger.Default.LogMode(logger.Info)
	} else {
		dbCfg.Logger = logger.Default.LogMode(logger.Silent)
	}
	dbt, dsn := c.DatabaseConnection()
	var err error
	switch dbt {
	case config.Psql:
		DB, err = gorm.Open(postgres.Open(dsn), dbCfg)
		if err != nil {
			return err
		}
	case config.Sqlite:
		DB, err = gorm.Open(sqlite.Open(dsn), dbCfg)
		if err != nil {
			return err
		}
	default:
		return ErrDBType
	}
	dbVer, initialized := migrationVersion()
	if initialized {
		if err = migratePre(dbVer); err != nil {
			return fmt.Errorf("pre-automigrate migration of database '%s' has failed: %w", dsn, err)
		}
	}
	if err = automigrate(); err != nil {
		return fmt.Errorf("auto migration of database '%s' has failed: %w", dsn, err)
	}
	if err = DB.SetupJoinTable(&History{}, "Links", &HistoryLink{}); err != nil {
		return fmt.Errorf("failed to setup join table for URL history: %w", err)
	}
	if initialized {
		if err = migratePost(dbVer); err != nil {
			return fmt.Errorf("post-automigrate migration of database '%s' has failed: %w", dsn, err)
		}
	} else {
		// Fresh database: AutoMigrate just created the latest schema, so record
		// the current version to avoid replaying historical migrations against it.
		DB.Save(&Database{Version: uint(len(migrations))})
	}
	return nil
}

func automigrate() error {
	return DB.AutoMigrate(
		&Database{},
		&History{},
		&Link{},
		&HistoryLink{},
		&IndexerVersion{},
		&User{},
		&CrawlJob{},
		&CrawlURL{},
		&DocumentVersion{},
	)
}

// Database represents the database version tracking table.
type Database struct {
	ID      uint `gorm:"primaryKey"`
	Version uint
}

// CommonFields contains fields common to all models.
type CommonFields struct {
	ID        uint       `gorm:"primary_key" json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type IndexerVersion struct {
	Version int `json:"version"`
}

func GetIndexerVersion() (int, error) {
	var r IndexerVersion
	if err := DB.Model(&IndexerVersion{}).First(&r).Error; err != nil {
		// No record yet — fresh installation; report as -1 so callers can
		// distinguish "never set" from version 0.
		return -1, nil
	}
	return r.Version, nil
}

func SetIndexerVersion(v int) error {
	var r IndexerVersion
	if err := DB.Model(&IndexerVersion{}).First(&r).Error; err != nil {
		r = IndexerVersion{v}
		return DB.Create(&r).Error
	}
	return DB.Model(&IndexerVersion{}).Where("version != ?", v).Update("version", v).Error
}
