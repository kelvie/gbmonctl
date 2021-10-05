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
	prop := flag.Int("p", 0x10, "Property to set, 0x10 is brightness (0-100), 0x12 is contrast(0-100), 0x87 is sharpness 1-10.")
	secondary := flag.Bool("s", false, "Use secondary command structure, properties are now: 0x0b for low blue light (0-10), 0x69 for KVM switching (0-1)")
	val :=  flag.Int("v", 50, "Value to set proeprty to")


	dryrun := flag.Bool("n", false, "Dry run: test commands and print instead")
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	// Buf is actually 192 bytes, but we need one for the report id
	buf := make([]byte, 193)

	buf[0] = 0
	copy(buf[1:], []byte{0x40, 0xc6})
	copy(buf[1+6:], []byte{0x20, 0, 0x6e, 0, 0x80})


	var preamble []byte

	if *secondary {
		preamble = []byte{0x51, 0x85, 0x03, 0xe0}
	} else {
		preamble = []byte{0x51, 0x84, 0x03}
	}

	msg := []byte{byte(*prop), 0, byte(*val)}

	copy(buf[1+0x40:], append(preamble, msg...))

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
