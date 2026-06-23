package models

import "time"

type HotspotRecord struct {
	ID         int64     `db:"id" json:"id"`
	Username   string    `db:"username" json:"username"`
	Mac        string    `db:"mac" json:"mac"`
	Address    string    `db:"address" json:"address"`
	Uptime     string    `db:"uptime" json:"uptime"`
	BytesIn    int64     `db:"bytes_in" json:"bytes_in"`
	BytesOut   int64     `db:"bytes_out" json:"bytes_out"`
	PacketsIn  int64     `db:"packets_in" json:"packets_in"`
	PacketsOut int64     `db:"packets_out" json:"packets_out"`
	IdleTime   string    `db:"idle_time" json:"idle_time"`
	Server     string    `db:"server" json:"server"`
	CollectedAt time.Time `db:"collected_at" json:"collected_at"`
}
