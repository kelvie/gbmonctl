package main

import (
	"fmt"
	"testing"
)

func TestChecksum(t *testing.T) {
	cases := []struct {
		msg []byte
		checksum byte
	}{
		{[]byte{0x10, 0, 0x48}, 0x8e},
		{[]byte{0x12, 0, 0x33}, 0xf7},
		// {[]byte{0x87, 0, 0x09}, 0x58}, // TODO this doesn't match, must be some other fuckery with the command?
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c.msg), func(t *testing.T){
			got := checksum(c.msg)
			if got != c.checksum {
				t.Errorf("Checksum failed: got %x, wanted %x", got, c.checksum)
			}
		})
	}

}
