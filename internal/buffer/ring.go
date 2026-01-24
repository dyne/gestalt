package buffer

type Ring[T any] struct {
	entries []T
	start   int
	count   int
}

func NewRing[T any](size int) *Ring[T] {
	if size <= 0 {
		size = 1
	}
	return &Ring[T]{
		entries: make([]T, size),
	}
}

func (r *Ring[T]) Add(entry T) {
	if r == nil || len(r.entries) == 0 {
		return
	}

	if r.count < len(r.entries) {
		index := (r.start + r.count) % len(r.entries)
		r.entries[index] = entry
		r.count++
		return
	}

	r.entries[r.start] = entry
	r.start = (r.start + 1) % len(r.entries)
}

func (r *Ring[T]) Len() int {
	if r == nil {
		return 0
	}
	return r.count
}

func (r *Ring[T]) List() []T {
	if r == nil || r.count == 0 {
		return nil
	}

	out := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		index := (r.start + i) % len(r.entries)
		out[i] = r.entries[index]
	}
	return out
}
