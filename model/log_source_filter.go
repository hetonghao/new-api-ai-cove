package model

import "gorm.io/gorm"

type LogSourceFilters struct {
	WebSocket bool
	FromTurbo bool
}

func applyLogSourceFilters(tx *gorm.DB, filters LogSourceFilters) *gorm.DB {
	if filters.WebSocket {
		tx = tx.Where(`(logs.other LIKE ? OR logs.other LIKE ? OR logs.other LIKE ? OR logs.other LIKE ?)`,
			`%"transport":"websocket"%`,
			`%"transport": "websocket"%`,
			`%"ws":true%`,
			`%"ws": true%`,
		)
	}
	if filters.FromTurbo {
		tx = tx.Where(`(logs.other LIKE ? OR logs.other LIKE ?)`,
			`%"client_source":"turbo"%`,
			`%"client_source": "turbo"%`,
		)
	}
	return tx
}
