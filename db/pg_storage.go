package database

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"log"
)

func NewClient(dbURL string) *Client {
	dataSource, err := pq.ParseURL(dbURL)
	if err != nil {
		log.Fatalf("DB URI parse error: %v\n", err)
		panic(err)
	}

	db, err := sqlx.Open("postgres", dataSource)
	if err != nil {
		log.Fatalf("DB Open error: %v\n", err)
		panic(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("DB Ping error: %v\n", err)
		panic(err)
	}
	return &Client{
		db: db,
	}
}

type Client struct {
	db *sqlx.DB
}

type UserAccount struct {
	UserID  sql.NullInt64  `db:"user_id" json:"user_id"`
	Account sql.NullString `db:"account" json:"account"`
}

func (c *Client) GetAccount(userID string) (string, error) {
	userAccounts := []UserAccount{}
	if err := c.db.Select(&userAccounts, `SELECT user_id, account FROM user_accounts WHERE user_id = $1`, userID); err != nil {
		log.Printf("DB query error %v\n", err)
		return "", nil
	}
	if len(userAccounts) != 1 {
		log.Printf("No user account found")
		return "", nil
	}
	return userAccounts[0].Account.String, nil
}

func (c *Client) SetAccount(userID, account string) error {
	row := c.db.QueryRow(`INSERT INTO user_accounts (user_id, account) VALUES ($1, $2)`, userID, account)
	return row.Scan()
}
