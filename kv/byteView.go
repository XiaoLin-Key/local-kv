package kv

type ByteView struct {
	b []byte
}

func (v ByteView) Len() int64 {
	return int64(len(v.b))
}

func (v ByteView) ByteSlice() []byte {
	clone := make([]byte, len(v.b))
	copy(clone, v.b)
	return clone
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
