package ringbuffer

import (
	"errors"
	"unsafe"

	"golang_project_note/gnet/internal"
	"golang_project_note/gnet/pool/bytebuffer"
)

const initSize = 1 << 12

var ErrIsEmpty = errors.New("ring-buffer is empty")

type RingBuffer struct {
	buf  []byte
	size int
	// 将数组转换成ring-buffer
	mask int
	// r 指向可读的那一位
	r int
	// w 指向可写的那一位
	w int
	// isEmpty
	isEmpty bool
}

func New(size int) *RingBuffer {
	if size == 0 {
		return &RingBuffer{isEmpty: true}
	}
	size = internal.CeilToPowerOfTwo(size)
	return &RingBuffer{
		buf:     make([]byte, size),
		size:    size,
		mask:    size - 1,
		isEmpty: true,
	}
}

// not move "read"
func (r *RingBuffer) LazyRead(len int) (head []byte, tail []byte) {
	if r.isEmpty || len <= 0 {
		return
	}

	if r.r < r.w {
		n := r.w - r.r
		if n > len {
			n = len
		}
		head = r.buf[r.r : r.r+n]
		return
	}

	n := r.size - r.r + r.w
	if n > len {
		n = len
	}

	if r.r+n < r.size {
		head = r.buf[r.r : r.r+n]
	} else {
		head = r.buf[r.r:]
		c := n - (r.size - r.r)
		tail = r.buf[:c]
	}

	return
}

// not move "read"
func (r *RingBuffer) LazyReadAll() (head []byte, tail []byte) {
	if r.isEmpty {
		return
	}

	if r.w > r.r {
		head = r.buf[r.r:r.w]
		return
	}

	head = r.buf[r.r:]
	if r.w != 0 {
		tail = r.buf[:r.w]
	}

	return
}

// 可读的byte长度
func (r *RingBuffer) Length() int {
	if r.r == r.w {
		if r.isEmpty {
			return 0
		}
		return r.size
	}

	if r.w > r.r {
		return r.w - r.r
	}

	return r.size - r.r + r.w
}

func (r *RingBuffer) Free() int {
	if r.r == r.w {
		if r.isEmpty {
			return r.size
		}
		return 0
	}

	if r.w < r.r {
		return r.r - r.w
	}
	return r.size - r.w + r.r
}

func (r *RingBuffer) Cap() int {
	return r.size
}

func (r *RingBuffer) Len() int {
	return len(r.buf)
}

func (r *RingBuffer) Reset() {
	r.r = 0
	r.w = 0
	r.isEmpty = true
}

func (r *RingBuffer) Shift(n int) {
	if n <= 0 {
		return
	}

	if n < r.Length() {
		r.r = (r.r + n) & r.mask
		if r.r == r.w {
			r.isEmpty = true
		}
	} else {
		r.Reset()
	}
}

func (r *RingBuffer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	if r.isEmpty {
		return 0, ErrIsEmpty
	}

	if r.w > r.r {
		n = r.w - r.r
		if n > len(p) {
			n = len(p)
		}
		copy(p, r.buf[r.r:r.r+n])
		r.r = r.r + n
		if r.r == r.w {
			r.isEmpty = true
		}
		return
	}

	n = r.size - r.r + r.w
	if n > len(p) {
		n = len(p)
	}

	if r.r+n < r.size {
		copy(p, r.buf[r.r:r.r+n])
	} else {
		c1 := r.size - r.r
		copy(p, r.buf[r.r:])
		c2 := n - c1
		copy(p[c1:], r.buf[:c2])
	}
	r.r = (r.r + n) & r.mask
	if r.r == r.w {
		r.isEmpty = true
	}

	return
}

func (r *RingBuffer) ReadByte() (b byte, err error) {
	if r.isEmpty {
		return 0, ErrIsEmpty
	}
	b = r.buf[r.r]
	r.r++
	if r.r == r.size {
		r.r = 0
	}
	if r.r == r.w {
		r.isEmpty = true
	}
	return b, err
}

func (r *RingBuffer) Write(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		return 0, nil
	}

	free := r.Free()
	if n > free {
		r.malloc(n - free)
	}

	if r.w >= r.r {
		c1 := r.size - r.w
		if c1 >= n {
			copy(r.buf[r.w:], p)
			r.w += n
		} else {
			copy(r.buf[r.w:], p[:c1])
			c2 := n - c1
			copy(r.buf, p[c1:])
			r.w = c2
		}
	} else {
		// 上面已经保证了足够的空间
		copy(r.buf[r.w:], p)
		r.w += n
	}

	if r.w == r.size {
		r.w = 0
	}
	r.isEmpty = false

	return n, err
}

func (r *RingBuffer) WriteByte(c byte) error {
	if r.Free() < 1 {
		r.malloc(1)
	}
	r.buf[r.w] = c
	r.w++

	if r.w == r.size {
		r.w = 0
	}
	r.isEmpty = false
	return nil
}

// string 转 byte数组的优化
func (r *RingBuffer) WriteString(s string) (n int, err error) {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	buf := *(*[]byte)(unsafe.Pointer(&h))
	return r.Write(buf)
}

// 将所有可读的byte保存到ByteBuffer中返回
func (r *RingBuffer) ByteBuffer() *bytebuffer.ByteBuffer {
	if r.isEmpty {
		return nil
	} else if r.w == r.r {
		bb := bytebuffer.Get()
		_, _ = bb.Write(r.buf[r.r:])
		_, _ = bb.Write(r.buf[:r.w])
		return bb
	}

	bb := bytebuffer.Get()
	if r.w > r.r {
		_, _ = bb.Write(r.buf[r.r:r.w])
		return bb
	}

	_, _ = bb.Write(r.buf[r.r:])
	if r.w != 0 {
		_, _ = bb.Write(r.buf[:r.w])
	}

	return bb
}

// 将所有可读的byte + 给到的b 保存到bytebuffer中返回
func (r *RingBuffer) WithByteBuffer(b []byte) *bytebuffer.ByteBuffer {
	if r.isEmpty {
		return &bytebuffer.ByteBuffer{B: b}
	} else if r.w == r.r {
		bb := bytebuffer.Get()
		_, _ = bb.Write(r.buf[r.r:])
		_, _ = bb.Write(r.buf[:r.w])
		_, _ = bb.Write(b)
		return bb
	}

	bb := bytebuffer.Get()
	if r.w > r.r {
		_, _ = bb.Write(r.buf[r.r:r.w])
		_, _ = bb.Write(b)
		return bb
	}

	_, _ = bb.Write(r.buf[r.r:])
	if r.w != 0 {
		_, _ = bb.Write(r.buf[:r.w])
	}
	_, _ = bb.Write(b)

	return bb
}

func (r *RingBuffer) IsFull() bool {
	return r.r == r.w && !r.isEmpty
}

func (r *RingBuffer) IsEmpty() bool {
	return r.isEmpty
}

// malloc 之后，r 指针会重新指向开头
func (r *RingBuffer) malloc(cap int) {
	var newCap int
	if r.size == 0 && initSize >= cap {
		newCap = initSize
	} else {
		newCap = internal.CeilToPowerOfTwo(r.size + cap)
	}
	newBuf := make([]byte, newCap)
	oldLen := r.Length()
	_, _ = r.Read(newBuf)
	r.r = 0
	r.w = oldLen
	r.size = newCap
	r.mask = newCap - 1
	r.buf = newBuf
}
