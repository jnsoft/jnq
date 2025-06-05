package sqlpqueue

import (
	"testing"
	"time"

	. "github.com/jnsoft/jnq/src/testhelper"
)

func TestPriorityQueue(t *testing.T) {

	channel := 10
	t.Run("integer min priority queue", func(t *testing.T) {

		pq := NewSqLitePQueue("", "", true)
		defer pq.ResetQueue()

		err := pq.Enqueue("item1", 1, channel, time.Now())
		AssertNoError(t, err)

		err = pq.Enqueue("item2", 2, channel, time.Now())
		AssertNoError(t, err)

		size, err := pq.Size(channel)
		AssertNoError(t, err)
		AssertEqual(t, size, 2)

		item, err := pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item1")

		item, err = pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item2")

		isEmpty, err := pq.IsEmpty(channel)
		AssertNoError(t, err)
		AssertTrue(t, isEmpty)
	})

	t.Run("string min priority queue", func(t *testing.T) {
		pq := NewSqLitePQueue("", "", true)
		defer pq.ResetQueue()

		err := pq.Enqueue("itemA", 1, channel, time.Now())
		AssertNoError(t, err)

		err = pq.Enqueue("itemB", 2, channel, time.Now())
		AssertNoError(t, err)

		item, err := pq.Peek(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "itemA")

		item, err = pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "itemA")

		item, err = pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "itemB")
	})

	t.Run("float min priority queue", func(t *testing.T) {
		pq := NewSqLitePQueue("", "", true)
		defer pq.ResetQueue()

		err := pq.Enqueue("item1", 1.1, channel, time.Now())
		AssertNoError(t, err)

		err = pq.Enqueue("item2", 2.2, channel, time.Now())
		AssertNoError(t, err)

		item, err := pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item1")

		item, err = pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item2")
	})

	t.Run("empty queue", func(t *testing.T) {
		pq := NewSqLitePQueue("", "", true)
		defer pq.ResetQueue()

		isEmpty, err := pq.IsEmpty(channel)
		AssertNoError(t, err)
		AssertTrue(t, isEmpty)

		size, err := pq.Size(channel)
		AssertNoError(t, err)
		AssertEqual(t, size, 0)
	})

	t.Run("enqueue and dequeue", func(t *testing.T) {
		pq := NewSqLitePQueue("", "", true)
		defer pq.ResetQueue()

		err := pq.Enqueue("item1", 1, channel, time.Now())
		AssertNoError(t, err)

		err = pq.Enqueue("item2", 2, channel, time.Now())
		AssertNoError(t, err)

		item, err := pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item1")

		item, err = pq.Dequeue(channel)
		AssertNoError(t, err)
		AssertEqual(t, item, "item2")
	})
}
