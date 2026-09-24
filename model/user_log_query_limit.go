package model

// GetUserLogQueryRateLimit reads the authoritative override so admin changes
// take effect on the next query on every instance, independently of auth caches.
func GetUserLogQueryRateLimit(userID int) (int, error) {
	var user User
	if err := DB.Select("log_query_rate_limit").First(&user, userID).Error; err != nil {
		return 0, err
	}
	if user.LogQueryRateLimit == nil {
		return 0, nil
	}
	return *user.LogQueryRateLimit, nil
}
