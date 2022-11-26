package game

import (
	"fmt"
	"strconv"
)

const (
	INTSIZE = 64
)

type BitMap []uint64 
type Position int
type Board struct {
	Turn int // 0: black 1: white
	discs int
	Boardlen int
	bitmapsz int
	unused_bits int
	black BitMap
	white BitMap
	h_sentinel BitMap
	v_sentinel BitMap
	s_sentinel BitMap
}

func IsDigit(c byte) bool {
	return byte('0') <= c && c <= byte('9')
}

func IsLetter(c byte) bool {
	return byte('a') <= c && c <= byte('z')
}

func NewBoardSFEN(boardlen int, sfen string) *Board {
	b := NewBoard(boardlen)

	disc_count := 0
	pos := 0
        i := 0
        for pos < boardlen*boardlen && i < len(sfen) {
		if IsDigit(sfen[i]) {
			nlen := 1
			for j := i+1; j < len(sfen); j++ {
				if IsDigit(sfen[j]) {
					nlen++
				} else {
					break
				}
			}
			num,_ := strconv.Atoi(string(sfen[i:i+nlen]))
			pos += num
			i += nlen
                } else if sfen[i] == byte('/') {
			i += 1
                } else if sfen[i] == byte('b') {
			set_bit(b.black, pos)
			pos += 1
			i += 1
			disc_count += 1
                } else if sfen[i] == byte('w') {
			set_bit(b.white, pos)
			pos += 1
			i += 1
			disc_count += 1
                } else {
			i += 1
                }
        }
	if pos == boardlen*boardlen {
		for ; i < len(sfen); i++ {
			if sfen[i] == byte('b') {
				b.Turn = 0
				break
			} else if sfen[i] == byte('w') {
				b.Turn = 1
				break
			}
		}
	}
	b.discs = disc_count
	return b
}

func NewBoard(boardlen int) *Board {
	bitmapsz := (boardlen*boardlen-1)/INTSIZE+1
	b := Board{
		Turn: 0,
		discs: 0,
		Boardlen: boardlen,
		bitmapsz: bitmapsz,
		unused_bits: INTSIZE*bitmapsz - boardlen*boardlen,
		black: make(BitMap, bitmapsz, bitmapsz),
		white: make(BitMap, bitmapsz, bitmapsz),
		h_sentinel: BitMap{},
		v_sentinel: BitMap{},
		s_sentinel: BitMap{},
	}
	b.h_sentinel = b.mk_horizontal_sentinel()
	b.v_sentinel = b.mk_vertical_sentinel()
	b.s_sentinel = b.mk_sides_sentinel()
	
	return &b
}

func (b *Board) DiscNum() int {
	return b.discs
}

func (b *Board) OR(x, y BitMap) BitMap {
	z := make(BitMap, b.bitmapsz, b.bitmapsz)
	for i := 0; i < b.bitmapsz; i++ {
		z[i] = x[i] | y[i]
	}
	return z
}

func (b *Board) AND(x, y BitMap) BitMap {
	z := make(BitMap, b.bitmapsz, b.bitmapsz)
	for i := 0; i < b.bitmapsz; i++ {
		z[i] = x[i] & y[i]
	}
	return z
}

func (b *Board) NOT(x BitMap) BitMap {
	y := make(BitMap, b.bitmapsz, b.bitmapsz)
	for i := 0; i < b.bitmapsz; i++ {
		y[i] = ^x[i]
	}
	return y
}

func (b *Board) SHR(a BitMap, n int) BitMap {
	c := make(BitMap, b.bitmapsz, b.bitmapsz)
	copy(c,a)

	if n >= INTSIZE {
		array_shift := n / INTSIZE
		bit_shift := n % INTSIZE
		copy(c[array_shift:], c[:len(c)-array_shift])
		for i := 0; i < array_shift; i++ {
			c[i] = uint64(0)
		}
		n = bit_shift
		if n == 0 {
			return c
		}
	}
	if n == 0 {
		panic("SHR panic")
	}

	for i := len(c)-1; i > 0; i-- {
		// n-bit shift to right 
		c[i] >>= n
		// the lower n-bits are 1
		var mask uint64 = (uint64(1) << n) - 1
		var data uint64 = (c[i-1] & mask) << (INTSIZE-n)
		c[i] |= data
	}
	c[0] >>= n
	if b.unused_bits != 0 {
		var mask2 uint64 = ^((uint64(1) << b.unused_bits) - 1)
		c[len(c)-1] &= mask2
	}
	return c
}

func (b *Board) SHL(a BitMap, n int) BitMap {
	c := make(BitMap, b.bitmapsz, b.bitmapsz)
	copy(c,a)

	if n >= INTSIZE {
		array_shift := n / INTSIZE
		bit_shift := n % INTSIZE
		copy(c[:len(c)-array_shift], c[array_shift:])
		for i := len(c)-array_shift; i < len(c); i++ {
			c[i] = uint64(0)
		}
		n = bit_shift
		if n == 0 {
			return c
		}
	}
	if n == 0 {
		panic("SHL panic")
	}
	for i := 0; i < len(c)-1; i++ {
		// n-bit shift to left
		c[i] <<= n
		// the upper n-bits are 1
		var mask uint64 = ^((uint64(1) << (INTSIZE-n)) - 1)
		var data uint64 = (c[i+1] & mask) >> (INTSIZE-n)
		c[i] |= data
	}
	if b.unused_bits != 0 {
		var mask2 uint64 = ^((uint64(1) << b.unused_bits) - 1)
		c[len(c)-1] &= mask2
	}
	c[len(c)-1] <<= n
	return c
}

func (b *Board) is_zero(a BitMap) bool {
	for i := 0; i < len(a); i++ {
		if a[i] != uint64(0) {
			return false
		}
	}
	return true
}

func set_bit(a BitMap, pos int) {
	a[pos / INTSIZE] |= uint64(0x8000_0000_0000_0000) >> (pos % INTSIZE)
}

func clear_bit(a BitMap, pos int) {
	a[pos / INTSIZE] &= ^(uint64(0x8000_0000_0000_0000) >> (pos % INTSIZE))
}

func (b *Board) open_spaces() []Position {
	bits := b.NOT(b.OR(b.black, b.white))
	var pms []Position
	for pos := 0; pos < b.Boardlen*b.Boardlen; pos++ {
		if (bits[pos / INTSIZE] & (uint64(0x8000_0000_0000_0000) >> (pos % INTSIZE))) != 0 {
			pms = append(pms, Position(pos))
		}
	}
	return pms
}

func (b *Board) Position2Str(pos Position) string {
	boardlen := b.Boardlen
	x := strconv.Itoa(int(pos) / boardlen + 1)

	yval := int(pos) % boardlen
	digit0 := yval / 26
	digit1 := yval % 26
	y := string(byte('a') + byte(digit1))
	if digit0 != 0 {
		y = string(byte('a') + byte(digit0) - 1) + y
	}
		
	return y + x
}

func (b *Board) move2BitMap(mv Position) BitMap {
	bits := make(BitMap, b.bitmapsz, b.bitmapsz)
	set_bit(bits, int(mv))
	return bits
}

func (b *Board) ToSFEN() string {
	var sfen string
	space_count := 0
        for pos := 0; pos < b.Boardlen*b.Boardlen; pos++ {
		if is_bit_on(b.black, pos) {
			if space_count != 0 {
				sfen += strconv.Itoa(space_count)
				space_count = 0
			}
			sfen += "b"
		} else if is_bit_on(b.white, pos) {
			if space_count != 0 {
				sfen += strconv.Itoa(space_count)
				space_count = 0
			}
			sfen += "w"
		} else {
			space_count++
		}
	}
	if space_count != 0 {
		sfen += strconv.Itoa(space_count)
	}
	if b.IsBlackTurn() {
		sfen += " b"
	} else {
		sfen += " w"
	}
	return sfen
}

func is_bit_on(bits BitMap, pos int) bool {
	return (bits[pos/INTSIZE] & (uint64(0x8000_0000_0000_0000) >> (pos % INTSIZE))) != 0
}

func (b *Board) PrintBoard() {
	s := "++++++++++ PRINT GAME ++++++++++\n"
	for x := 0; x < b.Boardlen; x++ {
		for y := 0; y < b.Boardlen; y++ {
			pos := x*b.Boardlen + y
			if is_bit_on(b.black,pos) {
				s += "B|"
			} else if is_bit_on(b.white,pos) {
				s += "W|"
			} else {
				s += " |"
			}
		}
		s += "\n"
	}
	fmt.Println(s)
	s2 := fmt.Sprintf("Black: %d  White: %d  ", b.CountBlack(), b.CountWhite())
	if b.IsBlackTurn() {
		s2 += "Turn: Black\n\n"
	} else {
		s2 += "Turn: White\n\n"
	}
	fmt.Println(s2)
}

func (b *Board) mk_horizontal_sentinel() BitMap {
	var bits BitMap
	for i := 0; i < b.bitmapsz; i++ {
		bits = append(bits, ^uint64(0))
	}
	for y := 0; y < b.Boardlen; y++ {
		pos0 := y*b.Boardlen + 0
		pos1 := y*b.Boardlen + b.Boardlen - 1
		clear_bit(bits, pos0)
		clear_bit(bits, pos1)
	}
	return bits
}

func (b *Board) mk_vertical_sentinel() BitMap {
	var bits BitMap
	for i := 0; i < b.bitmapsz; i++ {
		bits = append(bits, ^uint64(0))
	}
	for y := 0; y < b.Boardlen; y++ {
		pos0 := 0*b.Boardlen + y
		pos1 := (b.Boardlen-1)*b.Boardlen + y
		clear_bit(bits, pos0)
		clear_bit(bits, pos1)
	}
	return bits
}

func (b *Board) mk_sides_sentinel() BitMap {
	var bits BitMap
	for i := 0; i < b.bitmapsz; i++ {
		bits = append(bits, ^uint64(0))
	}
	for x := 0; x < b.Boardlen; x++ {
		pos0 := x*b.Boardlen + 0
		pos1 := x*b.Boardlen + b.Boardlen - 1
		clear_bit(bits, pos0)
		clear_bit(bits, pos1)
	}
	for y := 0; y < b.Boardlen; y++ {
		pos0 := 0*b.Boardlen + y
		pos1 := (b.Boardlen-1)*b.Boardlen + y
		clear_bit(bits, pos0)
		clear_bit(bits, pos1)
	}
	return bits
}

func (b *Board) IsLegalMove(mv Position) bool {
	var legals BitMap
	if b.IsBlackTurn() {
		legals = b.LegalMovesBits(b.black, b.white)
	} else {
		legals = b.LegalMovesBits(b.white, b.black)
	}
	if mv != -1 {
		move := b.move2BitMap(mv)
		if b.is_zero(b.AND(move, legals)) {
			return false
		} else {
			return true
		}
	} else {
		if b.is_zero(legals) {
			return true
		} else {
			return false
		}
	}
}

func (b *Board) LegalMoves() []Position {
	var legals BitMap
	if b.IsBlackTurn() {
		legals = b.LegalMovesBits(b.black, b.white)
	} else {
		legals = b.LegalMovesBits(b.white, b.black)
	}
	var pms []Position
	for x := 0; x < b.bitmapsz; x++ {
		if legals[x] != 0 {
			for y := 0; y < INTSIZE; y++ {
				if legals[x] & (uint64(0x8000_0000_0000_0000) >> y) != 0 {
					pms = append(pms, Position(x*INTSIZE + y))
				}
			}
		}
	}
	return pms
}

func (b *Board) IsBlackTurn() bool {
	if b.Turn == 0 {
		return true
	} else {
		return false
	}
}

func printbit(bits BitMap, boardlen int, xx []int, yy []int) {
	s := ""
	for x := xx[0]; x < xx[1]; x++ {
		for y := yy[0]; y < yy[1]; y++ {
			pos := x*boardlen + y
			if is_bit_on(bits, pos) {
				s += "o|"
			} else {
				s += ".|"
			}
		}
		s += "\n"
	}
	fmt.Println(s)
}

func (b *Board) LegalMovesBits(my_pos, op_pos BitMap) BitMap {
	opens := b.NOT(b.OR(my_pos,op_pos))
	mask_V := b.AND(op_pos, b.v_sentinel)
	mask_H := b.AND(op_pos, b.h_sentinel)
	mask_S := b.AND(op_pos, b.s_sentinel)
	shift_len_V := b.Boardlen
	shift_len_H := 1
	shift_len_DU := b.Boardlen - 1
	shift_len_DD := b.Boardlen + 1	

	legals := make(BitMap, b.bitmapsz, b.bitmapsz)
	var tmp BitMap

	// UP
	tmp = b.AND(mask_V, b.SHL(my_pos,shift_len_V))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_V, b.SHL(tmp,shift_len_V)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHL(tmp,shift_len_V)))

	// DOWN
	tmp = b.AND(mask_V, b.SHR(my_pos,shift_len_V))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_V, b.SHR(tmp,shift_len_V)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHR(tmp,shift_len_V)))

	// RIGHT
	tmp = b.AND(mask_H, b.SHR(my_pos,shift_len_H))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_H, b.SHR(tmp,shift_len_H)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHR(tmp,shift_len_H)))

	// LEFT
	tmp = b.AND(mask_H, b.SHL(my_pos,shift_len_H))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_H, b.SHL(tmp,shift_len_H)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHL(tmp,shift_len_H)))

	// RIGHT UP
	tmp = b.AND(mask_S, b.SHL(my_pos,shift_len_DU))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHL(tmp,shift_len_DU)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHL(tmp,shift_len_DU)))

	// LEFT DOWN
	tmp = b.AND(mask_S, b.SHR(my_pos,shift_len_DU))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHR(tmp,shift_len_DU)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHR(tmp,shift_len_DU)))

	// RIGHT DOWN
	tmp = b.AND(mask_S, b.SHR(my_pos,shift_len_DD))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHR(tmp,shift_len_DD)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHR(tmp,shift_len_DD)))
	
	// LEFT UP
	tmp = b.AND(mask_S, b.SHL(my_pos,shift_len_DD))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHL(tmp,shift_len_DD)))
	}
	legals = b.OR(legals, b.AND(opens, b.SHL(tmp,shift_len_DD)))

	return legals
}

func (b *Board) duplicate() *Board {
	b2 := &Board{
		Turn: b.Turn,
		discs: b.discs,
		Boardlen: b.Boardlen,
		bitmapsz: b.bitmapsz,
		unused_bits: b.unused_bits,
		black: make(BitMap, b.bitmapsz,b.bitmapsz),
		white: make(BitMap, b.bitmapsz,b.bitmapsz),
		h_sentinel: b.h_sentinel,
		v_sentinel: b.v_sentinel,
		s_sentinel: b.s_sentinel,
	}
	copy(b2.black, b.black)
	copy(b2.white, b.white)
	return b2
}

func (b *Board) Move(mv Position) *Board {
	b2 := b.duplicate()
	if mv != -1 {
		move := b2.move2BitMap(mv)
		flipped_discs := b2.flip(mv)
		if b2.IsBlackTurn() {
			b2.white = b2.AND(b2.white, b2.NOT(flipped_discs))
			b2.black = b2.OR(b2.black, flipped_discs)
			b2.black = b2.OR(b2.black, move)
		} else {
			b2.black = b2.AND(b2.black, b2.NOT(flipped_discs))
			b2.white = b2.OR(b2.white, flipped_discs)
			b2.white = b2.OR(b2.white, move)
		}
		b2.discs += 1
	}
	b2.Turn = 1 - b2.Turn
	return b2
}

func (b *Board) MoveUpdate(mv Position) {
	if mv != -1 {
		move := b.move2BitMap(mv)
		flipped_discs := b.flip(mv)
		if b.IsBlackTurn() {
			b.white = b.AND(b.white, b.NOT(flipped_discs))
			b.black = b.OR(b.black, flipped_discs)
			b.black = b.OR(b.black, move)
		} else {
			b.black = b.AND(b.black, b.NOT(flipped_discs))
			b.white = b.OR(b.white, flipped_discs)
			b.white = b.OR(b.white, move)
		}
		b.discs += 1
	}
	b.Turn = 1 - b.Turn
}

func (b *Board) IsGameOver() bool {
	bits0 := b.LegalMovesBits(b.black, b.white)
	if b.is_zero(bits0) {
		bits1 := b.LegalMovesBits(b.white, b.black)
		if b.is_zero(bits1) {
			return true
		}
	}
	return false
}

func PopCountUInt64(i uint64) (c int) {
	i -= (i >> 1) & 0x5555555555555555
	i = (i>>2)&0x3333333333333333 + i&0x3333333333333333
	i += i >> 4
	i &= 0x0f0f0f0f0f0f0f0f
	i *= 0x0101010101010101
	return int(i >> 56)
}

func (b *Board) CountBlack() int {
	return b.count_bits(b.black)
}

func (b *Board) CountWhite() int {
	return b.count_bits(b.white)
}

func (b *Board) count_bits(bits BitMap) int {
	count := 0
	for i := 0; i < len(bits); i++ {
		count += PopCountUInt64(bits[i])
	}
	return count
}

func (b *Board) flip(mv Position) BitMap {
	move := b.move2BitMap(mv)
	my_pos := make(BitMap, b.bitmapsz, b.bitmapsz)
	op_pos := make(BitMap, b.bitmapsz, b.bitmapsz)

	if b.IsBlackTurn() {
		copy(my_pos, b.black)
		copy(op_pos, b.white)
	} else {
		copy(my_pos, b.white)
		copy(op_pos, b.black)
	}

	flips := make(BitMap, b.bitmapsz, b.bitmapsz)
	mask_V := b.AND(op_pos, b.v_sentinel)
	mask_H := b.AND(op_pos, b.h_sentinel)
	mask_S := b.AND(op_pos, b.s_sentinel)
	shift_len_V := b.Boardlen
	shift_len_H := 1
	shift_len_DU := b.Boardlen - 1
	shift_len_DD := b.Boardlen + 1	
	var tmp BitMap
	
	// UP
	tmp = b.AND(mask_V, b.SHL(move,shift_len_V))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_V, b.SHL(tmp,shift_len_V)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHL(tmp,shift_len_V))) {
		flips = b.OR(flips, tmp)
	}

	// DOWN
	tmp = b.AND(mask_V, b.SHR(move,shift_len_V))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_V, b.SHR(tmp,shift_len_V)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHR(tmp,shift_len_V))) {
		flips = b.OR(flips, tmp)
	}

	// RIGHT
	tmp = b.AND(mask_H, b.SHR(move,shift_len_H))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_H, b.SHR(tmp,shift_len_H)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHR(tmp,shift_len_H))) {
		flips = b.OR(flips, tmp)
	}

	// LEFT
	tmp = b.AND(mask_H, b.SHL(move,shift_len_H))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_H, b.SHL(tmp,shift_len_H)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHL(tmp,shift_len_H))) {
		flips = b.OR(flips, tmp)
	}

	// RIGHT UP
	tmp = b.AND(mask_S, b.SHL(move,shift_len_DU))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHL(tmp,shift_len_DU)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHL(tmp,shift_len_DU))) {
		flips = b.OR(flips, tmp)
	}

	// LEFT DOWN
	tmp = b.AND(mask_S, b.SHR(move,shift_len_DU))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHR(tmp,shift_len_DU)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHR(tmp,shift_len_DU))) {
		flips = b.OR(flips, tmp)
	}

	// RIGHT DOWN
	tmp = b.AND(mask_S, b.SHR(move,shift_len_DD))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHR(tmp,shift_len_DD)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHR(tmp,shift_len_DD))) {
		flips = b.OR(flips, tmp)
	}
	
	// LEFT UP
	tmp = b.AND(mask_S, b.SHL(move,shift_len_DD))
	for i := 0; i < b.Boardlen-3; i++ {
		tmp = b.OR(tmp, b.AND(mask_S, b.SHL(tmp,shift_len_DD)))
	}
	if !b.is_zero(b.AND(my_pos, b.SHL(tmp,shift_len_DD))) {
		flips = b.OR(flips, tmp)
	}

	return flips
}

func MakeInitialSFEN(boardlen int) string {
	sfen0 := strconv.Itoa(boardlen*(boardlen/2-1) + (boardlen/2-1))
	sfen1 := "wb"
	sfen2 := strconv.Itoa(boardlen-2)
	sfen3 := "bw"
	sfen4 := strconv.Itoa(boardlen*((boardlen+1)/2-1) + (boardlen+1)/2-1)
	sfen5 := " b"
	return sfen0 + sfen1 + sfen2 + sfen3 + sfen4 + sfen5
}
