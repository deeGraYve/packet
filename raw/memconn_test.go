package raw

import (
	"fmt"
	"testing"
	"time"
)

func Test_bufferedPacketConn_ReadFrom(t *testing.T) {
	a, b := NewBufferedConn()

	sent := []byte("test")
	buffer := make([]byte, 32)
	count := 0

	go func(t *testing.T) {
		for {
			if _, _, err := b.ReadFrom(buffer); err != nil {
				panic(err)
			}
			count++
		}
	}(t)

	fmt.Println("going to write")
	a.WriteTo(sent, nil)
	a.WriteTo(sent, nil)
	a.WriteTo(sent, nil)
	time.Sleep(time.Millisecond * 5)
	if count != 3 {
		t.Fatal("error in read ", count)
	}

}
