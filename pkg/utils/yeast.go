package utils

import (
	"fmt"
	"sync"
	"time"
)

var (
	alphabet = [64]byte{
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J',
		'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T',
		'U', 'V', 'W', 'X', 'Y', 'Z', 'a', 'b', 'c', 'd',
		'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n',
		'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x',
		'y', 'z', '-', '_',
	}
	charToIndex = func() [256]int8 {
		var table [256]int8
		for i := range table {
			table[i] = -1
		}
		for i, char := range alphabet {
			table[char] = int8(i)
		}
		return table
	}()
)

const length = int64(64)

type Yeast struct {
	mu   sync.Mutex
	seed int64
	prev string
}

func NewYeast() *Yeast {
	return &Yeast{}
}

func (y *Yeast) Encode(num int64) string {
	if num < 0 {
		num = -num
	}
	if num == 0 {
		return "0"
	}

	var buf [11]byte
	i := len(buf)
	for num > 0 {
		i--
		buf[i] = alphabet[num%length]
		num /= length
	}
	return string(buf[i:])
}

func (y *Yeast) Decode(str string) (int64, error) {
	if len(str) == 0 {
		return 0, fmt.Errorf("yeast: empty string")
	}
	var decoded int64
	for i := 0; i < len(str); i++ {
		idx := charToIndex[str[i]]
		if idx < 0 {
			return 0, fmt.Errorf("yeast: invalid character %q at position %d", str[i], i)
		}
		decoded = decoded*length + int64(idx)
	}
	return decoded, nil
}

func (y *Yeast) Yeast() string {
	// The timestamp and seed must advance together, including across milliseconds.
	y.mu.Lock()
	defer y.mu.Unlock()

	now := y.Encode(time.Now().UnixMilli())

	if now != y.prev {
		y.seed = 0
		y.prev = now
		return now
	}

	id := now + "." + y.Encode(y.seed)
	y.seed++
	return id
}

var DefaultYeast = NewYeast()

func YeastDate() string {
	return DefaultYeast.Yeast()
}
