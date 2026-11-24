package main

// Reading values back from the monitor.
//
// The Realtek HID device (0bda:1100) is an I2C bridge to the monitor's DDC/CI
// interface. A read is three steps:
//
//  1. Output report "I2C write" (40 c6) carrying a DDC/CI Get VCP Feature
//     request.
//  2. Output report "I2C read" (40 d6) asking the bridge to fetch the reply.
//  3. GetInputReport to collect the reply. The bridge never sends input
//     reports on its own, so a plain Read() just times out.
//
// Protocol details from https://github.com/rimexe0/megabyte (re/PROTOCOL.md).

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/sstallion/go-hid"
)

const (
	vendorID  = 0x0bda
	productID = 0x1100

	// ddcReplyLen is how many bytes we ask the bridge to read back.
	ddcReplyLen = 36
)

// bridgeReport builds an output report: report ID 0 followed by the 192-byte
// report. Offsets within the 192-byte report:
//
//	0-1   command: 40 c6 = I2C write, 40 d6 = I2C read
//	2     0x51 (host address) for reads
//	6     length of the I2C transfer
//	8-10  6e <00 = write, 01 = read> 80
//	0x40  DDC/CI payload (writes only)
func bridgeReport(read bool, length int, payload []byte) []byte {
	buf := make([]byte, 193)
	r := buf[1:]
	r[0] = 0x40
	r[1] = 0xc6
	if read {
		r[1] = 0xd6
		r[2] = 0x51
		r[9] = 0x01
	}
	r[6] = byte(length)
	r[8] = 0x6e
	r[10] = 0x80
	copy(r[0x40:], payload)
	return buf
}

// getVCPPayload builds a DDC/CI Get VCP Feature request (opcode 0x01).
// Gigabyte's vendor codes (0xe0xx) are sent as two bytes. The trailing
// checksum is the XOR of the payload bytes.
func getVCPPayload(code uint16) []byte {
	msg := []byte{0x51, 0, 0x01}
	if code > 0xff {
		msg = binary.BigEndian.AppendUint16(msg, code)
	} else {
		msg = append(msg, byte(code))
	}
	msg[1] = byte(0x80 + len(msg) - 2)
	var cs byte
	for _, b := range msg {
		cs ^= b
	}
	return append(msg, cs)
}

// errStale means the bridge handed back its previous reply, which is what
// happens for codes that can be set but not read.
var errStale = errors.New("property cannot be read (monitor did not answer)")

// parseGetReply finds the DDC/CI Get VCP reply in an input report and returns
// the current and maximum values.
//
// The reply is "6e 8N 02 <result> <code...> [type] <maxH maxL> <curH curL> <chk>".
// The values are always the last four bytes of the N-byte payload. For
// two-byte Gigabyte codes the monitor drops the type byte so the payload stays
// at 8 bytes, which is why the offsets are taken from the end.
func parseGetReply(b []byte, code uint16) (cur, max uint16, err error) {
	for i := 0; i+3 < len(b); i++ {
		if b[i] != 0x6e || b[i+1]&0x80 == 0 || b[i+2] != 0x02 {
			continue
		}
		plen := int(b[i+1] & 0x7f)
		p := b[i+2:]
		if plen < 6 || len(p) < plen {
			continue
		}
		echo := uint16(p[2])
		if code > 0xff {
			echo = binary.BigEndian.Uint16(p[2:])
		}
		if echo != code {
			return 0, 0, fmt.Errorf("code %#x: %w", code, errStale)
		}
		if p[1] != 0 {
			return 0, 0, fmt.Errorf("code %#x: monitor reports it is unsupported", code)
		}
		return binary.BigEndian.Uint16(p[plen-2:]), binary.BigEndian.Uint16(p[plen-4:]), nil
	}
	return 0, 0, fmt.Errorf("code %#x: no reply found", code)
}

type monitor struct{ dev *hid.Device }

func openMonitor() (*monitor, error) {
	if err := hid.Init(); err != nil {
		return nil, err
	}
	dev, err := hid.OpenFirst(vendorID, productID)
	if err != nil {
		return nil, fmt.Errorf("opening monitor (%04x:%04x): %w", vendorID, productID, err)
	}
	return &monitor{dev}, nil
}

func (m *monitor) Close() {
	m.dev.Close()
	hid.Exit()
}

// readVCP reads one VCP code. The bridge occasionally drops transfers that
// come too close together, so it waits between steps and retries.
func (m *monitor) readVCP(code uint16) (cur, max uint16, err error) {
	payload := getVCPPayload(code)
	wr := bridgeReport(false, len(payload), payload)
	rd := bridgeReport(true, ddcReplyLen, nil)
	in := make([]byte, 193)

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		if _, err = m.dev.Write(wr); err != nil {
			continue
		}
		time.Sleep(150 * time.Millisecond)
		if _, err = m.dev.Write(rd); err != nil {
			continue
		}
		time.Sleep(100 * time.Millisecond)

		var n int
		if n, err = m.dev.GetInputReport(in); err != nil {
			continue
		}
		if os.Getenv("GBMONCTL_DEBUG") != "" {
			log.Printf("code %#x reply:\n%s", code, hex.Dump(in[:min(n, 32)]))
		}
		// A stale reply can also mean our request was dropped, so retry it
		// like any other failure.
		if cur, max, err = parseGetReply(in[:n], code); err == nil {
			return
		}
	}
	return
}

// getValue prints only the current value of one code, so scripts can use
// $(gbmonctl -get brightness). Errors go to stderr with a non-zero exit.
func getValue(code uint16, dryrun bool) {
	if dryrun {
		payload := getVCPPayload(code)
		log.Println("Would have sent:\n" + hex.Dump(bridgeReport(false, len(payload), payload)))
		log.Println("Then:\n" + hex.Dump(bridgeReport(true, ddcReplyLen, nil)))
		return
	}
	m, err := openMonitor()
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	cur, _, err := m.readVCP(code)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cur)
}

// getAll prints a table of every known property.
func getAll(props []Property) {
	m, err := openMonitor()
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	for i, p := range props {
		if i > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		cur, max, err := m.readVCP(p.Value)
		if err != nil {
			fmt.Printf("%-16s %v\n", p.Name, err)
			continue
		}
		fmt.Printf("%-16s %-6d (max %d)\n", p.Name, cur, max)
	}
}

// parseCode resolves a -get argument: a property name, or a raw VCP code in
// decimal or 0x-prefixed hex.
func parseCode(s string, props map[string]Property) (uint16, error) {
	if p, ok := props[s]; ok {
		return p.Value, nil
	}
	n, err := strconv.ParseUint(s, 0, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("unknown property: %s", s)
	}
	return uint16(n), nil
}
