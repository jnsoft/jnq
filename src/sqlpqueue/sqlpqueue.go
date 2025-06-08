package sqlpqueue

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jnsoft/jngo/pqueue"
	"github.com/jnsoft/jnq/src/priorityqueue"
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
            NotBefore INTEGER NOT NULL,
			Reserved INTEGER NOT NULL
			ReservedId TEXT NULL
        );`
	selectSQL = "SELECT * FROM %s WHERE Reserved = 0 and Channel = ? and NotBefore <= ? ORDER BY Prio %s LIMIT 1"
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
	insertSQL := fmt.Sprintf("INSERT INTO %s (Prio, Obj, Channel, NotBefore, Reserved) VALUES (?, ?, ?, ?, ?)", pq.table)
	_, err = db.Exec(insertSQL, prio, obj, channel, nb, 0)
	return err
}

func (pq *SqLitePQueue) Dequeue(channel int) (string, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return "", err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	order := "ASC"
	if !pq.isMinQueue {
		order = "DESC"
	}

	selectSQL := fmt.Sprintf(selectSQL, pq.table, order)
	row := tx.QueryRow(selectSQL, channel, time.Now().Unix())

	var id int
	var obj string
	err = row.Scan(&id, &obj)
	if err == sql.ErrNoRows {
		return "", errors.New(pqueue.EMPTY_QUEUE)
	} else if err != nil {
		return "", err
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE Id = ?", pq.table)
	_, err = tx.Exec(deleteSQL, id)
	if err != nil {
		return "", err
	}

	return obj, nil

	/*
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
	*/
}

func (pq *SqLitePQueue) DequeueWithReservation(channel int) (string, string, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return "", "", err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	order := "ASC"
	if !pq.isMinQueue {
		order = "DESC"
	}
	selectSQL := fmt.Sprintf(selectSQL, pq.table, order)
	row := tx.QueryRow(selectSQL, channel, time.Now().Unix())

	var id int
	var obj string
	err = row.Scan(&id, &obj)
	if err == sql.ErrNoRows {
		return "", "", errors.New(pqueue.EMPTY_QUEUE)
	} else if err != nil {
		return "", "", err
	}

	reservationId := uuid.New().String()
	updateSQL := fmt.Sprintf("UPDATE %s SET Reserved = 1, ReservedId = ? WHERE Id = ?", pq.table)
	_, err = tx.Exec(updateSQL, reservationId, id)
	if err != nil {
		return "", "", err
	}

	return obj, reservationId, nil

}

func (pq *SqLitePQueue) ConfirmReservation(reservationId string) (bool, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return false, err
	}
	defer db.Close()

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE Reserved = 1 and ReservedId = ?", pq.table)
	res, err := db.Exec(deleteSQL, reservationId)

	if err != nil {
		return false, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (pq *SqLitePQueue) RequeueExpiredReservations(timeout time.Duration) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	requeueTime := time.Now().Add(-timeout).Unix()
	requeueSQL := fmt.Sprintf("UPDATE %s SET Reserved = 0, ReservedId = NULL WHERE Reserved = 1 AND NotBefore <= ?", pq.table)
	_, err = db.Exec(requeueSQL, requeueTime)
	if err != nil {
		panic(err)
	}
}

func (pq *SqLitePQueue) Size(channel int) (int, error) {
	db, err := sql.Open("sqlite3", pq.connectionString)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE Reserved = 0 and Channel = ? and NotBefore <= ?", pq.table)
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

	checkSQL := fmt.Sprintf("SELECT 1 FROM %s WHERE Reserved = 0 and Channel = ? and NotBefore <= ? LIMIT 1", pq.table)
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
var _ priorityqueue.IPriorityQueue = (*SqLitePQueue)(nil)
