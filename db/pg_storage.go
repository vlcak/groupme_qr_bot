package database

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"log"
	"time"
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

type GroupmeAccount struct {
	UserID  sql.NullInt64  `db:"user_id" json:"user_id"`
	Account sql.NullString `db:"account" json:"account"`
}

type UserAccount struct {
	Account sql.NullString `db:"account" json:"account"`
	Name    sql.NullString `db:"name" json:"name"`
}

type Payment struct {
	Account sql.NullString `db:"account" json:"account"`
	Name    sql.NullString `db:"name" json:"name"`
}

func (c *Client) GetGroupmeAccount(userID string) (string, error) {
	var account string
	if err := c.db.Get(&account, `SELECT account FROM groupme_accounts WHERE user_id = $1`, userID); err != nil {
		log.Printf("DB query error %v\n", err)
		return "", err
	}
	if account == "" {
		log.Printf("No user account found")
	}
	return account, nil
}

func (c *Client) SetGroupmeAccount(userID, account string) error {
	row := c.db.QueryRow(`INSERT INTO groupme_accounts (user_id, account) VALUES ($1, $2)`, userID, account)
	return row.Scan()
}

func (c *Client) GetName(account string) (string, error) {
	var name string
	if err := c.db.Get(&name, `SELECT name FROM user_accounts WHERE account = $1`, account); err != nil {
		log.Printf("DB query error %v\n", err)
		return "", err
	}
	if name == "" {
		log.Printf("No names found for account %s", account)
	}
	return name, nil
}

func (c *Client) GetLastPaymentOrder() (int, error) {
	var lastOrder int
	if err := c.db.Get(&lastOrder, `SELECT accounted_order FROM payments ORDER BY accounted_order DESC LIMIT 1`); err != nil {
		log.Printf("DB query error %v\n", err)
		return 0, err
	}
	if lastOrder == 0 {
		log.Printf("No payments found")
		return 0, errors.New("No payments found")
	}
	return lastOrder, nil
}

func (c *Client) StorePayment(name, account string, amount, order int, timestamp time.Time) error {
	row := c.db.QueryRow(`INSERT INTO payments (name, account, amount, accounted_order, accounted_at) VALUES ($1, $2, $3, $4, $5)`, name, account, amount, order, timestamp)
	return row.Scan()
}
