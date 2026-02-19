package store

type RingBuffer struct {
	items   []string
	levels  []uint8
	start   int
	size    int
	version uint64
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingBuffer{items: make([]string, capacity), levels: make([]uint8, capacity)}
}

func (r *RingBuffer) Append(line string) {
	r.AppendWithLevel(line, 0)
}

func (r *RingBuffer) AppendWithLevel(line string, level uint8) {
	if len(r.items) == 0 {
		return
	}
	idx := (r.start + r.size) % len(r.items)
	r.items[idx] = line
	r.levels[idx] = level
	r.version++
	if r.size < len(r.items) {
		r.size++
		return
	}
	r.start = (r.start + 1) % len(r.items)
}

func (r *RingBuffer) Items() []string {
	out := make([]string, r.size)
	for i := 0; i < r.size; i++ {
		idx := (r.start + i) % len(r.items)
		out[i] = r.items[idx]
	}
	return out
}

func (r *RingBuffer) ItemsWithLevels() ([]string, []uint8) {
	lines := make([]string, r.size)
	levels := make([]uint8, r.size)
	for i := 0; i < r.size; i++ {
		idx := (r.start + i) % len(r.items)
		lines[i] = r.items[idx]
		levels[i] = r.levels[idx]
	}
	return lines, levels
}

func (r *RingBuffer) Len() int {
	return r.size
}

func (r *RingBuffer) Clear() {
	for i := range r.items {
		r.items[i] = ""
		r.levels[i] = 0
	}
	r.start = 0
	r.size = 0
	r.version++
}

func (r *RingBuffer) Version() uint64 {
	return r.version
}
