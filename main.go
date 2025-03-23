package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/sstallion/go-hid"
)

func checksum(msg []byte) byte {
	var checksumLower uint8 = 0x6
	for _, b := range msg {
		checksumLower ^= b
	}
	checksumLower &= 0xf
	return checksumLower + (msg[len(msg)-1]^0xc0)&0xf0
}

type Property struct {
	Name        string
	Description string
	Min         byte
	Max         byte
	Value       uint16
}

func main() {
	properties := []Property{
		{
			Name:  "brightness",
			Min:   0,
			Max:   100,
			Value: 0x10,
		},
		{
			Name:  "contrast",
			Min:   0,
			Max:   100,
			Value: 0x12,
		},
		{
			Name:  "sharpness",
			Min:   0,
			Max:   10,
			Value: 0x87,
		},
		{
			Name: "volume",
			Min: 0,
			Max: 100,
			Value: 0x62,

		},
		{
			Name:        "low-blue-light",
			Description: "Blue light reduction. 0 means no reduction.",
			Min:         0,
			Max:         10,
			Value:       0xe00b,
		},
		{
			Name:  "kvm-switch",
			Description: "Switch KVM to device 0 or 1",
			Min:   0,
			Max:   1,
			Value: 0xe069,
		},
		{
			Name:        "colour-mode",
			Description: "0 is cool, 1 is normal, 2 is warm, 3 is user-defined.",
			Min:         0,
			Max:         3,
			Value:       0xe003,
		},
		{
			Name:        "count",
			Description: "Set counter to specific value",
			Min:         0,
			Max:         99,
			Value:       0xe02a,
		},
		{
			Name:        "counter",
			Description: "1 to show gaming counter on screen, 0 to hide it",
			Min:         0,
			Max:         1,
			Value:       0xe028,
		},
		{
			Name:        "crosshair",
			Description: "0 is off, 1-4 switches between crosshairs",
			Min:         0,
			Max:         4,
			Value:       0xe037,
		},
		{
			Name:        "rgb-red",
			Description: "Red value -- only works if colour-mode is set to 3",
			Min:         0,
			Max:         100,
			Value:       0xe004,
		},
		{
			Name:        "rgb-green",
			Description: "Green value -- only works if colour-mode is set to 3",
			Min:         0,
			Max:         100,
			Value:       0xe005,
		},
		{
			Name:        "rgb-blue",
			Description: "Blue value -- only works if colour-mode is set to 3",
			Min:         0,
			Max:         100,
			Value:       0xe006,
		},
		{
			Name:        "timer",
			Description: "0 is off, 1 is start or reset",
			Min:         0,
			Max:         1,
			Value:       0xe023,
		},
		{
			Name:        "timer-pause",
			Description: "0 is pause, 1 resume",
			Min:         0,
			Max:         1,
			Value:       0xe027,
		},
	}

	propMap := make(map[string]Property)
	propHelp := []string{}
	for _, p := range properties {
		propMap[p.Name] = p
		propText := fmt.Sprintf("\t%s (%d-%d)", p.Name, p.Min, p.Max)
		if p.Description != "" {
			propText = propText + "\n\t\t" + p.Description
		}
		propHelp = append(propHelp, propText)
	}

	prop := flag.String("prop", "", "Property to set. Available properties: \n"+strings.Join(propHelp, "\n"))
	propNum := flag.Uint("propNum", 0, "Property number to set instead of -prop")
	val := flag.Int("val", -1, "Value to set property to")
	dryrun := flag.Bool("n", false, "Dry run: test commands and print instead")

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	errExit := func(str string) {
		fmt.Printf("ERROR: %s\n\n", str)
		flag.Usage()
		os.Exit(1)
	}

	if *prop == "" && *propNum == 0 {
		// TODO: launch a repl or gui with tab completion instead here, or
		// something like fish_config
		errExit("-prop or -propNum is required")
	}

	if *val == -1 {
		errExit("-val is required")
	}

	if *propNum != 0 && *prop != "" {
		errExit("Specify only one of -prop or -propNum")
	}

	var prop16 uint16

	if *propNum == 0 {
		found, ok := propMap[*prop]
		if !ok {
			errExit(fmt.Sprintf("Unknown property: %s", *prop))
		}
		if *val > int(found.Max) || *val < int(found.Min) {
			errExit(fmt.Sprintf("Value %d for property %s is not within range: %d-%d", *val, found.Name, found.Min, found.Max))
		}
		prop16 = found.Value
	} else {
		prop16 = uint16(*propNum)
	}

	// Buf is actually 192 bytes, but we need one for the report id
	buf := make([]byte, 193)

	buf[0] = 0
	copy(buf[1:], []byte{0x40, 0xc6})
	copy(buf[1+6:], []byte{0x20, 0, 0x6e, 0, 0x80})

	var preamble []byte
	msg := []byte{}

	if prop16 > 0xff {
		msg = append(msg, byte(prop16>>8))
		prop16 &= 0xff
	}

	msg = append(msg, []byte{byte(prop16), 0, byte(*val)}...)

	// TODO: 0x01 is read, 0x03 is write
	preamble = []byte{0x51, byte(0x81 + len(msg)), 0x03}

	copy(buf[1+0x40:], append(preamble, msg...))

	if *dryrun {
		log.Println("Would have sent:\n" + hex.Dump(buf))
		return
	}

	err := hid.Init()

	if err != nil {
		log.Fatal(err)
	}

	dev, err := hid.OpenFirst(0x0bda, 0x1100)
	if err != nil {
		log.Fatal(err)
	}

	// TODO: get current value and nicely transition to the expected value like in
	// TODO: read a value if "v" not specified, I think the value is in the byte
	// 0xa of the response if we do a read
	_, err = dev.Write(buf)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Property set.")
}
