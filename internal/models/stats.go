package models

type UserStat struct {
	UserID    string `db:"user_id" json:"user_id"`
	Username  string `db:"username" json:"username"`
	Mac       string `db:"mac" json:"mac"`
	Generated int    `db:"generated" json:"generated"`
	Logins    int    `db:"logins" json:"logins"`
	Failed    int    `db:"failed" json:"failed"`
}
