package iso

import (
	"fmt"
	"strconv"
)

/* This struct operates with MSB as the first bit of the data set */
type BitSet struct {
	data uint64
}

func (b *BitSet) GetBit(index int) bool {
	return (b.data>>(63-index))&1 == 1
}

func (b *BitSet) SetBit(index int, val bool) {
	mask := uint64(1) << (63 - index)
	// clear bit
	b.data &^= mask
	// set bit if needed
	if val {
		b.data |= mask
	}
}

func (b *BitSet) SetString(s string, base int) error {
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return err
	}
	b.data = v
	return nil
}
func (b *BitSet) BitLen() int {
	return 64
}

func (b BitSet) ToHex() string {
	return fmt.Sprintf("%016X", b.data)
}
