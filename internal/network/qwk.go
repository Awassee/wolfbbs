package network

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func writeQWKBundle(path string, p Packet) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()

	control := []string{
		defaultIfBlank(strings.TrimSpace(p.SourceBBS), "WolfBBS"),
		p.Exported.UTC().Format(time.RFC3339),
		strconv.Itoa(len(p.Messages)),
	}
	if err := writeZipText(zw, "CONTROL.DAT", strings.Join(control, "\n")+"\n"); err != nil {
		return err
	}

	headers := make([]map[string]interface{}, 0, len(p.Messages))
	var bodies strings.Builder
	for idx, msg := range p.Messages {
		headers = append(headers, map[string]interface{}{
			"index":      idx + 1,
			"board_id":   msg.BoardID,
			"board":      msg.Board,
			"conference": msg.Conference,
			"from_user":  msg.FromUserID,
			"to_user":    msg.ToUserID,
			"to_handle":  msg.ToHandle,
			"subject":    msg.Subject,
			"parent_id":  msg.ParentID,
			"thread_id":  msg.ThreadID,
		})
		bodies.WriteString(fmt.Sprintf("::MSG %d\n", idx+1))
		bodies.WriteString(strings.TrimSpace(msg.Body))
		bodies.WriteString("\n::ENDMSG\n")
	}
	if len(headers) > 0 {
		raw, err := json.Marshal(headers)
		if err != nil {
			return err
		}
		if err := writeZipText(zw, "HEADERS.DAT", string(raw)+"\n"); err != nil {
			return err
		}
	}
	if err := writeZipText(zw, "MESSAGES.DAT", bodies.String()); err != nil {
		return err
	}
	rawMsgs, err := json.MarshalIndent(p.Messages, "", "  ")
	if err != nil {
		return err
	}
	if err := writeZipText(zw, "MESSAGES.JSON", string(rawMsgs)+"\n"); err != nil {
		return err
	}
	rawPacket, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeZipText(zw, "WOLFBBS.JSON", string(rawPacket)+"\n")
}

func readQWKBundle(path string) (Packet, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Packet{}, err
	}
	defer reader.Close()

	files := map[string]*zip.File{}
	for _, file := range reader.File {
		name := strings.ToUpper(strings.TrimSpace(filepath.Base(file.Name)))
		if name == "" {
			continue
		}
		files[name] = file
	}

	if file, ok := files["WOLFBBS.JSON"]; ok {
		raw, err := readZipFile(file, 8*1024*1024)
		if err != nil {
			return Packet{}, err
		}
		var packet Packet
		if err := json.Unmarshal(raw, &packet); err == nil {
			packet.Format = FormatQWK
			return packet, nil
		}
	}

	packet := Packet{
		Version:   1,
		Format:    FormatQWK,
		Exported:  time.Now().UTC(),
		SourceBBS: "QWK",
		Messages:  []PacketMessage{},
	}
	if file, ok := files["MESSAGES.JSON"]; ok {
		raw, err := readZipFile(file, 8*1024*1024)
		if err != nil {
			return Packet{}, err
		}
		var messages []PacketMessage
		if err := json.Unmarshal(raw, &messages); err == nil {
			packet.Messages = messages
		}
	}
	if len(packet.Messages) == 0 {
		if file, ok := files["HEADERS.DAT"]; ok {
			raw, err := readZipFile(file, 8*1024*1024)
			if err != nil {
				return Packet{}, err
			}
			var headers []map[string]interface{}
			if err := json.Unmarshal(raw, &headers); err == nil {
				for _, row := range headers {
					packet.Messages = append(packet.Messages, PacketMessage{
						Board:      headerString(row, "board"),
						Conference: headerString(row, "conference"),
						ToHandle:   headerString(row, "to_handle"),
						Subject:    headerString(row, "subject"),
						Body:       "",
						BoardID:    headerInt64(row, "board_id"),
						FromUserID: headerInt64(row, "from_user"),
						ToUserID:   headerInt64(row, "to_user"),
						ParentID:   headerInt64(row, "parent_id"),
						ThreadID:   headerInt64(row, "thread_id"),
					})
				}
			}
		}
		if file, ok := files["MESSAGES.DAT"]; ok && len(packet.Messages) > 0 {
			raw, err := readZipFile(file, 8*1024*1024)
			if err != nil {
				return Packet{}, err
			}
			blocks := parseQWKMessageBodies(string(raw))
			for idx := range packet.Messages {
				if body, ok := blocks[idx+1]; ok {
					packet.Messages[idx].Body = body
				}
			}
		}
	}
	if len(packet.Messages) == 0 {
		return Packet{}, fmt.Errorf("qwk packet did not include parseable messages")
	}
	return packet, nil
}

func writeZipText(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, body)
	return err
}

func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

func parseQWKMessageBodies(raw string) map[int]string {
	blocks := map[int]string{}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "::MSG ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		newline := strings.IndexByte(part, '\n')
		if newline <= 0 {
			continue
		}
		idRaw := strings.TrimSpace(part[:newline])
		id, err := strconv.Atoi(idRaw)
		if err != nil || id <= 0 {
			continue
		}
		bodyPart := part[newline+1:]
		if end := strings.Index(bodyPart, "\n::ENDMSG"); end >= 0 {
			bodyPart = bodyPart[:end]
		}
		blocks[id] = strings.TrimSpace(bodyPart)
	}
	return blocks
}

func headerString(row map[string]interface{}, key string) string {
	if row == nil {
		return ""
	}
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func headerInt64(row map[string]interface{}, key string) int64 {
	value := headerString(row, key)
	if value == "" {
		return 0
	}
	var out int64
	_, _ = fmt.Sscan(value, &out)
	return out
}

func defaultIfBlank(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
