package concurrency

import (
	"fmt"
	"sync"

	uuid "github.com/satori/go.uuid"
)

// BCasterOption - option to initialize the bcaster
type BCasterOption func(*BCaster)

// BCaster - Is a broadcaster that allows to send events of type concurrency.Event to registered listeners using
// go concurrency patterns. Listeners are chan interfaces{} allowing for go concurrent communication.
// Closure of BCaster is handle by a concurrency.DoneHandler that allows to control they way a set of go routines
// are closed in order to prevent deadlocks and unwanted behaviour
// It detects when listeners are done and performs the required cleanup to ensure that events are sent to the
// active listeners.
type BCaster struct {
	id           string
	listeners    *SortedMap
	closed       bool
	listenerLock *sync.RWMutex
	MsgType      string
	doneHandler  *DoneHandler
	lock         *sync.RWMutex
	transformFn  func(b *BCaster, input interface{}) interface{}
}

// NewBCaster - Constructor
func NewBCaster(dh *DoneHandler, MsgType string, opts ...BCasterOption) *BCaster {
	id := uuid.NewV4().String()
	b := &BCaster{
		id:           id,
		doneHandler:  dh,
		listeners:    NewSortedMap(),
		closed:       false,
		MsgType:      MsgType,
		listenerLock: &sync.RWMutex{},
		lock:         &sync.RWMutex{},
	}
	b.transformFn = defaultBCasterTransformFn
	for _, opt := range opts {
		opt(b)
	}
	go b.doneRn()
	return b
}

// ID - retrieves the Id of the Bcaster
func (b *BCaster) ID() string {
	return b.id
}

// BCasterTransformFn - option to add a function to transform the output into
// the desired output structure to the BCaster
func BCasterTransformFn(fn func(b *BCaster, input interface{}) interface{}) BCasterOption {
	return func(b *BCaster) {
		b.transformFn = fn
	}
}

// AddListener - creates a listener as chan interface{} with a DoneHandler in order to manage its closure and pass it to the
// requestor so it can be used in order to consume events from the Bcaster
func (b *BCaster) AddListener(dh *DoneHandler) chan interface{} {
	b.listenerLock.Lock()
	defer b.listenerLock.Unlock()
	id := uuid.NewV4().String()

	listenerCh := OrDoneParamFn(dh.Done(), make(chan interface{}), b.RemoveListenerByKey, id)
	if !b.closed {
		b.listeners.Set(id, listenerCh)
		return listenerCh
	}
	return nil
}

// RemoveListenerByKey - Removes a listener by its key value
func (b *BCaster) RemoveListenerByKey(key interface{}) {

	b.listenerLock.Lock()
	b.listeners.Delete(key)
	b.listenerLock.Unlock()

}

// RemoveListener - removes a listener
func (b *BCaster) RemoveListener(listenerCh chan interface{}) {

	if key, ok := b.listeners.GetKeyByItem(listenerCh); ok {
		b.RemoveListenerByKey(key)
	}
	b.listenerLock.Lock()
	CloseChannel(listenerCh)
	b.listenerLock.Unlock()
}

// Broadcast - Transforms a message into a concurrency.Event and broadcasts it to all the active registered listeners
func (b *BCaster) Broadcast(msg interface{}) {

	closed := b.getClosed()
	b.lock.Lock()
	e := b.transformFn(b, msg)
	b.lock.Unlock()
	if !closed {
		b.listenerLock.RLock()
		for item := range b.listeners.Iter() {
			toNextItem := false
			listener := item.Value.(chan interface{})
			if listener == nil {
				toNextItem = true
				fmt.Printf("Broadcast - nil listerner\n")

			}
		loop:
			for !toNextItem {
				select {
				case listener <- e:
					toNextItem = true
					continue loop
				default:

				}
			}

		}
		b.listenerLock.RUnlock()
	}

}

// cleanListeners - Removes all the registered listeners
func (b *BCaster) cleanListeners() {
	for _, key := range b.listenersToSlice() {
		b.listenerLock.Lock()
		listenerCh, ok := b.listeners.Get(key)
		b.listenerLock.Unlock()
		b.RemoveListenerByKey(key)
		if ok {
			CloseChannel(listenerCh.(chan interface{}))
		}
	}
}

// close - Closes the BCaster
func (b *BCaster) close() {
	b.setClosed(true)
	fmt.Printf("Caster closed\n")
	b.cleanListeners()

}

// doneRn - Checks when the BCaster is done by listening to the closure of the DoneHandler.Done channel
func (b *BCaster) doneRn() {
	select {
	case <-b.doneHandler.Done():
		b.close()
	}
}

// setClosed - set the closed property
func (b *BCaster) setClosed(val bool) {
	b.lock.Lock()
	b.closed = val
	b.lock.Unlock()
}

// getClosed - get the closed property
func (b *BCaster) getClosed() bool {
	b.lock.Lock()
	c := b.closed
	b.lock.Unlock()
	return c
}

// listenersToSlice - Copies the listeners concurrency.SortedMap into a slice
func (b *BCaster) listenersToSlice() []interface{} {
	s := []interface{}{}
	for item := range b.listeners.Iter() {
		s = append(s, item.Key)
	}
	return s
}

// createEventFromMsg - creates a concurrency.Event from a concurrency.Message
// func (b *BCaster) createEventFromMsg(msg *Message) *Event {
// 	b.lock.Lock()
// 	s := *NewSlice()
// 	e := &Event{
// 		InitMessage:       msg,
// 		InMessageSequence: s,
// 		OutMessage:        msg,
// 		Sequence:          0,
// 	}
// 	b.lock.Unlock()
// 	return e
// }

// BCasterEventTransformFn - Gets the bcaster and input message the output in the form of an event
func BCasterEventTransformFn(b *BCaster, input interface{}) interface{} {
	var msg *Message
	if input != nil {
		msg = input.(*Message)
	}
	s := *NewSlice()
	e := &Event{
		InitMessage:       msg,
		InMessageSequence: s,
		OutMessage:        msg,
		Sequence:          0,
	}
	return e
}

// defaultBCasterTransformFn - Gets the bcaster, input and result and outputs the input
func defaultBCasterTransformFn(b *BCaster, input interface{}) interface{} {
	return input
}
