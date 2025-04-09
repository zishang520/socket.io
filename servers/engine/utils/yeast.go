package utils

import (
	"sync/atomic"
	"time"
)

var (
	alphabet = [64]string{
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
		"K", "L", "M", "N", "O", "P", "Q", "R", "S", "T",
		"U", "V", "W", "X", "Y", "Z", "a", "b", "c", "d",
		"e", "f", "g", "h", "i", "j", "k", "l", "m", "n",
		"o", "p", "q", "r", "s", "t", "u", "v", "w", "x",
		"y", "z", "-", "_",
	}
	mapCharToIndex = map[byte]int64{
		'0': 0, '1': 1, '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9,
		'A': 10, 'B': 11, 'C': 12, 'D': 13, 'E': 14, 'F': 15, 'G': 16, 'H': 17, 'I': 18, 'J': 19,
		'K': 20, 'L': 21, 'M': 22, 'N': 23, 'O': 24, 'P': 25, 'Q': 26, 'R': 27, 'S': 28, 'T': 29,
		'U': 30, 'V': 31, 'W': 32, 'X': 33, 'Y': 34, 'Z': 35, 'a': 36, 'b': 37, 'c': 38, 'd': 39,
		'e': 40, 'f': 41, 'g': 42, 'h': 43, 'i': 44, 'j': 45, 'k': 46, 'l': 47, 'm': 48, 'n': 49,
		'o': 50, 'p': 51, 'q': 52, 'r': 53, 's': 54, 't': 55, 'u': 56, 'v': 57, 'w': 58, 'x': 59,
		'y': 60, 'z': 61, '-': 62, '_': 63,
	}
)

const length = int64(64)

type Yeast struct {
	seed atomic.Int64
	prev atomic.Value
}

func NewYeast() *Yeast {
	return &Yeast{}
}

func (y *Yeast) Encode(num int64) (encoded string) {
	if num == 0 {
		return alphabet[0]
	}

	for num > 0 {
		encoded = alphabet[num%length] + encoded
		num /= length
	}

	return encoded
}

func (y *Yeast) Decode(str string) int64 {
	var decoded int64
	for i := 0; i < len(str); i++ {
		decoded = decoded*length + mapCharToIndex[str[i]]
	}
	return decoded
}

func (y *Yeast) Yeast() string {
	now := y.Encode(time.Now().UnixMilli())

	prev, _ := y.prev.Load().(string)
	if now != prev {
		y.seed.Store(0)
		y.prev.Store(now)
		return now
	}

	return now + "." + y.Encode(y.seed.Add(1)-1)
}

var DefaultYeast = NewYeast()

func YeastDate() string {
	return DefaultYeast.Yeast()
}
