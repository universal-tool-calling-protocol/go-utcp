package streamresult

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
