package mempqueue

import (
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
