package transports

import "io"

type StreamResult interface {
	Next() (interface{}, error)
	Close() error
}

type SliceStreamResult struct {
	items   []any
	index   int
	closeFn func() error
}

func NewSliceStreamResult(items []any, closeFn func() error) *SliceStreamResult {
	return &SliceStreamResult{items: items, closeFn: closeFn}
}

func (sr *SliceStreamResult) Next() (any, error) {
	if sr.index >= len(sr.items) {
		return nil, io.EOF
	}
	item := sr.items[sr.index]
	sr.index++
	return item, nil
}

func (sr *SliceStreamResult) Close() error {
	if sr.closeFn != nil {
		return sr.closeFn()
	}
	return nil
}

// ChannelStreamResult adapts a <-chan any into a StreamResult.
type ChannelStreamResult struct {
	ch      <-chan any
	closeFn func() error
}

// NewChannelStreamResult constructs a StreamResult from a channel and a close function.
func NewChannelStreamResult(ch <-chan any, closeFn func() error) StreamResult {
	return &ChannelStreamResult{
		ch:      ch,
		closeFn: closeFn,
	}
}

// Next returns the next element from the channel or io.EOF if closed.
func (sr *ChannelStreamResult) Next() (any, error) {
	item, ok := <-sr.ch
	if !ok {
		return nil, io.EOF
	}
	// If the channel carries an error value, return it.
	if err, isErr := item.(error); isErr {
		return nil, err
	}
	return item, nil
}

// Close invokes the provided close function to terminate the stream.
func (sr *ChannelStreamResult) Close() error {
	return sr.closeFn()
}
