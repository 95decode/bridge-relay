package models

import "gorm.io/gorm"

type Block struct {
	gorm.Model
	Domain      uint8 `gorm:"uniqueIndex"`
	BlockNumber uint64
}
