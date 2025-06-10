package mempqueue

import (
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/jnsoft/jnq/src/testhelper"
)

func TestMemPQueue(t *testing.T) {

	channel := 10
	t.Run("basic operations", func(t *testing.T) {
		q := NewMemPQueue(true)

		// check queue is empty
		isEmpty, err := q.IsEmpty(channel)
		AssertNil(t, err)
		AssertTrue(t, isEmpty)

		// enqueue items
		q.Enqueue("item1", 1, channel, time.Time{})
		q.Enqueue("item2", 2, channel, time.Time{})
		q.Enqueue("item3", 3, channel, time.Time{})

		// check queue size
		size, err := q.Size(channel)
		AssertNil(t, err)
		AssertEqual(t, size, 3)

		// peek first item
		value, err := q.Peek(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item1")

		// dequeue first item
		value, err = q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item1")

		// dequeue second item
		value, err = q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item2")

		// dequeue third item
		value, err = q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item3")

		// check queue is empty
		isEmpty, err = q.IsEmpty(channel)
		AssertNil(t, err)
		AssertTrue(t, isEmpty)
	})

	t.Run("not before time", func(t *testing.T) {
		q := NewMemPQueue(true)

		// enqueue items with notBefore time
		q.Enqueue("item1", 1, channel, time.Now().Add(200*time.Millisecond))
		q.Enqueue("item2", 2, channel, time.Now().Add(500*time.Millisecond))
		q.Enqueue("item3", 3, channel, time.Time{})

		// check queue size
		size, err := q.Size(channel)
		AssertNil(t, err)
		AssertEqual(t, size, 1)

		// wait for notBefore times to pass
		time.Sleep(1 * time.Second)

		// process notBefore queue
		q.processNotBeforeQueue()

		// check queue size
		size, err = q.Size(channel)
		AssertNil(t, err)
		AssertEqual(t, size, 3)

		// dequeue items
		value, err := q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item1")

		value, err = q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item2")

		value, err = q.Dequeue(channel)
		AssertNil(t, err)
		AssertEqual(t, value, "item3")
	})

	t.Run("reset queue", func(t *testing.T) {
		q := NewMemPQueue(true)

		// enqueue items
		q.Enqueue("item1", 1, channel, time.Time{})
		q.Enqueue("item2", 2, channel, time.Time{})
		q.Enqueue("item3", 3, channel, time.Time{})

		// reset queue
		err := q.ResetQueue()
		AssertNil(t, err)

		// check queue is empty
		isEmpty, err := q.IsEmpty(channel)
		AssertNil(t, err)
		AssertTrue(t, isEmpty)
	})

}

func TestMemPQueuePersistence(t *testing.T) {

	snapshotFile, err := os.CreateTemp("", "deleteme-*.sav")
	AssertNoError(t, err)
	defer os.Remove(snapshotFile.Name())

	walFile, err := os.CreateTemp("", "deleteme-*.sav")
	AssertNoError(t, err)
	defer os.Remove(walFile.Name())

	snap := snapshotFile.Name()
	wal := walFile.Name()

	//snap = "snap.sav"
	//wal = "wal.sav"
	//defer os.Remove("snap.sav")
	//defer os.Remove("wal.sav")

	channel := 5

	// 1. Enqueue items and persist
	q := NewMemPQueuePersistent(true, snap, wal)
	AssertTrue(t, q != nil)
	q.Enqueue("persist1", 1, channel, time.Time{})
	q.Enqueue("persist2", 2, channel, time.Time{})
	q.Enqueue("persist3", 3, channel, time.Time{})
	//q.maybeCheckpoint() // TODO: test checkpoint

	// 2. Reload and check items

	q = NewMemPQueuePersistent(true, snap, wal)
	size, err := q.Size(channel)
	AssertNil(t, err)
	AssertEqual(t, size, 3)
	val, err := q.Dequeue(channel)
	AssertNil(t, err)
	AssertEqual(t, val, "persist1")
	val, err = q.Dequeue(channel)
	AssertNil(t, err)
	AssertEqual(t, val, "persist2")
	val, err = q.Dequeue(channel)
	AssertNil(t, err)
	AssertEqual(t, val, "persist3")

	// 3. Test reservation persistence

	q = NewMemPQueuePersistent(true, snap, wal)
	q.Enqueue("resitem", 1, channel, time.Time{})
	_, resId, err := q.DequeueWithReservation(channel)
	AssertNil(t, err)

	q = NewMemPQueuePersistent(true, snap, wal)
	AssertTrue(t, len(q.reserved) == 1)
	for _, res := range q.reserved {
		AssertEqual(t, res.Item.Obj, "resitem")
	}

	// 4. Test confirm persistence
	q.ConfirmReservation(resId)
	q = NewMemPQueuePersistent(true, snap, wal)
	AssertTrue(t, len(q.reserved) == 0)

	// 5. Test not-before persistence

	q = NewMemPQueuePersistent(true, snap, wal)
	q.Enqueue("futureitem", 1, channel, time.Now().Add(500*time.Millisecond))

	q = NewMemPQueuePersistent(true, snap, wal)
	size, err = q.Size(channel)
	AssertNil(t, err)
	AssertEqual(t, size, 0)
	time.Sleep(600 * time.Millisecond)
	size, err = q.Size(channel)
	AssertNil(t, err)
	AssertEqual(t, size, 1)
	val, err = q.Dequeue(channel)
	AssertNil(t, err)
	AssertEqual(t, val, "futureitem")

}

func TestMemPQueueSnapshot(t *testing.T) {

	snapshotFile, err := os.CreateTemp("", "deleteme-*.sav")
	AssertNoError(t, err)
	defer os.Remove(snapshotFile.Name())

	walFile, err := os.CreateTemp("", "deleteme-*.sav")
	AssertNoError(t, err)
	defer os.Remove(walFile.Name())

	snap := snapshotFile.Name()
	wal := walFile.Name()

	snap = "snap.sav"
	wal = "wal.sav"
	defer os.Remove("snap.sav")
	defer os.Remove("wal.sav")

	channel := 7
	no_of_messages := CHECKPOINT_COUNT*10 + 10

	q := NewMemPQueuePersistent(true, snap, wal)
	AssertTrue(t, q != nil)
	for i := 0; i < no_of_messages; i++ {
		q.Enqueue("persist"+strconv.Itoa(i), float64(i), channel, time.Time{})
	}

	size, err := q.Size(channel)
	AssertNil(t, err)
	AssertEqual(t, size, no_of_messages)

	q = NewMemPQueuePersistent(true, snap, wal)
	size, err = q.Size(channel)
	AssertNil(t, err)
	AssertEqual(t, size, no_of_messages)

}
