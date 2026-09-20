package model

// RelayMediaObject is a durable deletion ledger, not a user asset or quota entry.
// Persist before uploading, so interrupted requests leave recoverable cleanup work.
type RelayMediaObject struct {
	ID        string `gorm:"type:varchar(64);primaryKey"`
	Bucket    string `gorm:"type:varchar(255);not null"`
	Region    string `gorm:"type:varchar(64);not null"`
	ObjectKey string `gorm:"type:varchar(255);not null"`
	ExpiresAt int64  `gorm:"index;not null"`
}
