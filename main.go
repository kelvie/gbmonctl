package main

import (
	"encoding/hex"
	"flag"
	"log"

	"github.com/sstallion/go-hid"
)

func checksum(msg []byte) byte {
	var checksumLower uint8 = 0x6
	for _, b := range msg {
		checksumLower ^= b
	}
	checksumLower &= 0xf
	return checksumLower + (msg[len(msg)-1] ^ 0xc0) & 0xf0
}

func main() {
	prop := flag.Int("p", 0x10, "Property to set, 16 is brightness, 18 is contrast, 137 is sharpness")
	val :=  flag.Int("v", 50, "Value to set it to (0-100 for brightness and contrast, 1-10 for sharpness)")
	dryrun := flag.Bool("n", false, "Dry run: test commands and print instead")

	flag.Parse()

	// Buf is actually 192 bytes, but we need one for the report id
	buf := make([]byte, 193)

	buf[0] = 0
	copy(buf[1:], []byte{0x40, 0xc6})
	copy(buf[1+6:], []byte{0x20, 0, 0x6e, 0, 0x80})

	msg := []byte{byte(*prop), 0, byte(*val)}
	copy(buf[1+0x40:], []byte{0x51, 0x84, 0x03})

	msg = append(msg, checksum(msg))

	copy(buf[1+0x43:], msg)

	if *dryrun {
		log.Println("Would have sent:\n" + hex.Dump(buf))
		return
	}

	err := hid.Init();

	if err != nil {
		log.Fatal(err)
	}

	dev, err := hid.OpenFirst(0x0bda, 0x1100)
	if err != nil {
		log.Fatal(err)
	}

	_, err = dev.Write(buf)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Property set.")
}
