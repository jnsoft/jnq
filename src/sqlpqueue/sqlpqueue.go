package sqlpqueue

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jnsoft/jngo/pqueue"
	"github.com/jnsoft/jnq/src/mempqueue"
	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultTable            = "QueueItems"
	defaultConnectionString = "queue.db"
	createTableSQL          = `
        CREATE TABLE IF NOT EXISTS %s (
            Id INTEGER PRIMARY KEY AUTOINCREMENT,
            Prio DOUBLE NOT NULL,
            Obj TEXT NOT NULL,
			Channel INTEGER NOT NULL,
            NotBefore INTEGER NOT NULL
        );`
	selectSQL = "SELECT * FROM %s WHERE Channel = ? and NotBefore <= ? ORDER BY Prio %s LIMIT 1"
)

type SqLitePQueue struct {
	connectionString string
	table            string
	isMinQueue       bool
}

func NewSqLitePQueue(connectionString, table string, isMinQueue bool) *SqLitePQueue {
	if connectionString == "" {
		connectionString = defaultConnectionString
	}
	if table == "" {
		table = defaultTable
	}
	pq := &SqLitePQueue{
		connectionString: connectionString,
		table:            table,
		isMinQueue:       isMinQueue,
	}
	pq.initDb()
	return pq
}

func (pq *SqLitePQueue) Enqueue(obj string, prio float64, channel int, notBefore time.Time) error {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	nb := notBefore.Unix()
	insertSQL := fmt.Sprintf("INSERT INTO %s (Prio, Obj, Channel, NotBefore) VALUES (?, ?, ?, ?)", pq.table)
	_, err = db.Exec(insertSQL, prio, obj, channel, nb)
	return err
}

func (pq *SqLitePQueue) Dequeue(channel int) (string, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return "", err
	}
	defer db.Close()

	hasItem, id, item, err := pq.peek(channel)
	if err != nil {
		return "", err
	}
	if hasItem {
		deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE Id = ?", pq.table)
		_, err = db.Exec(deleteSQL, id)
		if err != nil {
			return "", err
		}
		return item, nil
	}
	return "", errors.New(pqueue.EMPTY_QUEUE)
}

func (pq *SqLitePQueue) Size(channel int) (int, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE Channel = ? and NotBefore <= ?", pq.table)
	row := db.QueryRow(countSQL, channel, time.Now().Unix())
	var count int
	err = row.Scan(&count)
	return count, err
}

func (pq *SqLitePQueue) IsEmpty(channel int) (bool, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return false, err
	}
	defer db.Close()

	checkSQL := fmt.Sprintf("SELECT 1 FROM %s WHERE Channel = ? and NotBefore <= ? LIMIT 1", pq.table)
	row := db.QueryRow(checkSQL, channel, time.Now().Unix())
	var exists int
	err = row.Scan(&exists)
	if err == sql.ErrNoRows {
		return true, nil
	}
	return false, err
}

func (pq *SqLitePQueue) Peek(channel int) (string, error) {
	hasItem, _, item, err := pq.peek(channel)
	if !hasItem {
		return "", errors.New(pqueue.EMPTY_QUEUE)
	}
	return item, err
}

func (pq *SqLitePQueue) ResetQueue() error {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	resetSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s; %s", pq.table, fmt.Sprintf(createTableSQL, pq.table))
	_, err = db.Exec(resetSQL)
	return err
}

func (pq *SqLitePQueue) peek(channel int) (bool, int, string, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return false, 0, "", err
	}
	defer db.Close()

	order := "ASC"
	if !pq.isMinQueue {
		order = "DESC"
	}
	selectSQL := fmt.Sprintf(selectSQL, pq.table, order)
	row := db.QueryRow(selectSQL, channel, time.Now().Unix())
	var id int
	var prio float64
	var obj string
	var ch int
	var notBefore int64
	err = row.Scan(&id, &prio, &obj, &ch, &notBefore)
	if err == sql.ErrNoRows {
		return false, 0, "", nil
	}
	return true, id, obj, err
}

func (pq *SqLitePQueue) initDb() {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	createTable := fmt.Sprintf(createTableSQL, pq.table)
	_, err = db.Exec(createTable)
	if err != nil {
		panic(err)
	}
}

// Ensure SqLitePQueue implements IPriorityQueue
var _ mempqueue.IPriorityQueue = (*SqLitePQueue)(nil)
