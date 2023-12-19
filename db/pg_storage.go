package database

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const (
	GOALIE  = "goalie"
	DEFENSE = "defense"
	FORWARD = "forward"
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

type Player struct {
	Id     sql.NullInt64  `db:"id" json:"id"`
	Name   sql.NullString `db:"name" json:"name"`
	Number sql.NullInt64  `db:"number" json:"number"`
	Post   sql.NullString `db:"post" json:"post"`
}

type BankAccount struct {
	Id       sql.NullInt64  `db:"id" json:"id"`
	PlayerId sql.NullInt64  `db:"player_id" json:"player_id"`
	Account  sql.NullString `db:"account" json:"account"`
}

type Payment struct {
	Account sql.NullString `db:"account" json:"account"`
	Name    sql.NullString `db:"name" json:"name"`
	Amount  sql.NullInt64  `db:"amount" json:"amount"`
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
	_, err := c.db.Exec(`INSERT INTO groupme_accounts (user_id, account) VALUES ($1, $2)`, userID, account)
	return err
}

func (c *Client) GetName(account string) (string, error) {
	var name string
	if err := c.db.Get(&name, `SELECT p.name FROM players AS p JOIN bank_accounts AS b ON p.id = b.player_id WHERE b.account = $1`, account); err != nil {
		log.Printf("DB query error %v\n", err)
		return "", err
	}
	if name == "" {
		log.Printf("No names found for account %s", account)
	}
	return name, nil
}

func (c *Client) GetPlayerByName(name string) (Player, error) {
	var player Player
	if err := c.db.Get(&player, `SELECT p.* FROM players AS p LEFT JOIN nicknames AS n ON p.id = n.player_id WHERE p.name = $1 OR n.nickname = $1`, name); err != nil {
		log.Printf("DB query error %v\n", err)
		return player, err
	}
	if !player.Id.Valid {
		log.Printf("No players found for %s", name)
	}
	return player, nil
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
	_, err := c.db.Exec(`INSERT INTO payments (name, account, amount, accounted_order, accounted_at) VALUES ($1, $2, $3, $4, $5)`, name, account, amount, order, timestamp)
	return err
}

func (c *Client) MarkPaymentProcessed(order int) error {
	_, err := c.db.Exec(`UPDATE payments SET processed_at = $2 WHERE accounted_order = $1`, order, time.Now())
	return err
}

func (c *Client) GetUnprocessedPayments() ([]Payment, error) {
	var payments []Payment
	if err := c.db.Select(&payments, `SELECT name, account, amount FROM payments WHERE accounted_at >= timestamp '2023-09-01 00:00:00' AND processed_at IS NULL ORDER BY accounted_order ASC`); err != nil {
		log.Printf("DB query error %v\n", err)
		return payments, err
	}
	return payments, nil
}

func (c *Client) StoreScheduleException(date, time string) error {
	_, err := c.db.Exec(`INSERT INTO schedule_exceptions (id, date, time) VALUES (nextval('schedule_exceptions_id_seq'), $1, $2)`, date, time)
	if err != nil {
		log.Printf("DB query error %v\n", err)
	}
	return err
}

func (c *Client) IsException(date, time string) (bool, error) {
	var count int
	if err := c.db.Get(&count, `SELECT COUNT(*) FROM schedule_exceptions WHERE (date = $1 AND time = $2) OR (date = $1 AND time = '')`, date, time); err != nil {
		log.Printf("DB query error %v\n", err)
		return false, err
	}
	return count > 0, nil
}
