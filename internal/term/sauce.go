package term

import (
	"encoding/binary"
	"strings"
)

const sauceRecordSize = 128

type SAUCE struct {
	Title    string
	Author   string
	Group    string
	Date     string
	FileSize uint32
	DataType byte
	FileType byte
	TInfo1   uint16
	TInfo2   uint16
	TInfo3   uint16
	TInfo4   uint16
	Comments byte
	Flags    byte
	FontName string
}

func ParseSAUCE(data []byte) (SAUCE, bool) {
	if len(data) < sauceRecordSize {
		return SAUCE{}, false
	}
	record := data[len(data)-sauceRecordSize:]
	if string(record[0:7]) != "SAUCE00" {
		return SAUCE{}, false
	}
	metadata := SAUCE{
		Title:    cleanFixedField(record[7:42]),
		Author:   cleanFixedField(record[42:62]),
		Group:    cleanFixedField(record[62:82]),
		Date:     cleanFixedField(record[82:90]),
		FileSize: binary.LittleEndian.Uint32(record[90:94]),
		DataType: record[94],
		FileType: record[95],
		TInfo1:   binary.LittleEndian.Uint16(record[96:98]),
		TInfo2:   binary.LittleEndian.Uint16(record[98:100]),
		TInfo3:   binary.LittleEndian.Uint16(record[100:102]),
		TInfo4:   binary.LittleEndian.Uint16(record[102:104]),
		Comments: record[104],
		Flags:    record[105],
		FontName: cleanFixedField(record[106:128]),
	}
	return metadata, true
}

func StripSAUCE(data []byte) []byte {
	metadata, ok := ParseSAUCE(data)
	if !ok {
		return data
	}
	end := len(data) - sauceRecordSize
	// Optional comment block marker is "COMNT" + 64 bytes per comment.
	commentBlockSize := int(metadata.Comments) * 64
	if commentBlockSize > 0 && end >= (5+commentBlockSize) {
		markerStart := end - (5 + commentBlockSize)
		if markerStart >= 0 && string(data[markerStart:markerStart+5]) == "COMNT" {
			end = markerStart
		}
	}
	if end < 0 {
		return data
	}
	return data[:end]
}

func cleanFixedField(value []byte) string {
	return strings.TrimSpace(strings.TrimRight(string(value), "\x00"))
}
