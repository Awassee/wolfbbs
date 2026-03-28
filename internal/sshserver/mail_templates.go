package sshserver

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/mailflow"
	"wolfbbs/internal/ui"
)

func (s *Server) composeMailFlow(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, currentUser *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func(), seed *mailflow.Template) error {
	if currentUser == nil {
		return fmt.Errorf("missing user")
	}
	if touch == nil {
		touch = func() {}
	}
	seedSubject := ""
	seedBody := ""
	seedUrgency := "normal"
	seedName := ""
	if seed != nil {
		seedName = strings.TrimSpace(seed.Name)
		seedSubject = strings.TrimSpace(seed.Subject)
		seedBody = strings.TrimSpace(seed.Body)
		if seed.Urgency != "" {
			seedUrgency = normalizeMailUrgencySSH(seed.Urgency)
		}
	}
	to := ""
	subject := seedSubject
	urgency := seedUrgency
	drawCompose := func(note string) {
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Mail Compose", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.RenderMailCompose(renderWidth, "Private Mail Compose", to, subject, urgency, seedName, note), ansiEnabled, encoding)
	}
	drawCompose("")
	toValue, err := s.promptRecipient(sess, reader, termWidth, renderWidth, handle, th, ansiEnabled, encoding, time24h, nodeLabel, touch, func() {
		drawCompose("")
	})
	if err != nil {
		return err
	}
	to = strings.TrimSpace(toValue)
	drawCompose("")
	if seedSubject != "" {
		_, _ = io.WriteString(sess, "Subject ["+seedSubject+"]: ")
	} else {
		_, _ = io.WriteString(sess, "Subject: ")
	}
	inputSubject, err := readLine(reader, 120)
	if err != nil {
		return err
	}
	touch()
	inputSubject = strings.TrimSpace(inputSubject)
	if inputSubject != "" {
		subject = inputSubject
	}
	drawCompose("")
	_, _ = io.WriteString(sess, "Urgency ["+seedUrgency+"]: ")
	inputUrgency, err := readLine(reader, 16)
	if err != nil {
		return err
	}
	touch()
	urgency = normalizeMailUrgencySSH(defaultIfBlankSSH(strings.TrimSpace(inputUrgency), seedUrgency))
	bodyNote := "Type your message below. Your text stays visible as you type."
	bodyPrompt := "Enter body, end with '.' on a line by itself."
	if seedBody != "" {
		bodyPrompt = "Loaded kit body. Add or edit lines, use /preview to inspect, '.' to send."
		bodyNote = "Reply kit text is preloaded. Add or trim lines before you send."
	}
	drawCompose(bodyNote)
	_, _ = io.WriteString(sess, "\r\n"+bodyPrompt+"\r\n")
	body, err := readMessageBodySeeded(sess, reader, 80, 4096, seedBody)
	if err != nil {
		return err
	}
	touch()
	body = strings.TrimSpace(body)
	if to == "" || subject == "" || body == "" {
		_, _ = io.WriteString(sess, "\r\nTo/subject/body are required. Press any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	subject = applyMailUrgencySSH(subject, urgency)
	var msg domain.PrivateMail
	msg.FromUserID = currentUser.ID
	msg.Subject = subject
	msg.Body = body
	if strings.Contains(to, "@") {
		msg.ExternalTo = &to
	} else {
		targetUser, err := s.auth.GetUser(to)
		if err != nil || targetUser == nil {
			suggestions := s.suggestHandles(to, handle, 5)
			_, _ = io.WriteString(sess, "\r\nUnknown recipient handle.")
			if len(suggestions) > 0 {
				_, _ = io.WriteString(sess, "\r\nTry: "+strings.Join(suggestions, ", "))
			}
			_, _ = io.WriteString(sess, "\r\nPress any key.")
			_, _ = readKey(reader)
			touch()
			return nil
		}
		msg.ToUserID = targetUser.ID
	}
	if err := s.mail.CreateMail(&msg); err != nil {
		_, _ = io.WriteString(sess, "\r\nCould not send mail: "+err.Error()+"\r\nPress any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	if seed != nil {
		_ = mailflow.RecordUse(s.admin, handle, seed.ID)
	}
	s.publishEvent("mail.sent", map[string]string{"user": handle, "to": to})
	_, _ = io.WriteString(sess, "\r\nMail sent.\r\n")
	return nil
}

func (s *Server) replyToMailFlow(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, currentUser *domain.User, source *domain.PrivateMail, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) error {
	if currentUser == nil || source == nil {
		return fmt.Errorf("missing reply context")
	}
	if touch == nil {
		touch = func() {}
	}
	subject := strings.TrimSpace(source.Subject)
	if subject == "" {
		subject = "Re: (no subject)"
	} else if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	var toUserID int64
	var externalTo *string
	if source.ToUserID == currentUser.ID {
		toUserID = source.FromUserID
	} else {
		toUserID = source.ToUserID
		if source.ExternalTo != nil {
			trimmed := strings.TrimSpace(*source.ExternalTo)
			if trimmed != "" {
				externalTo = &trimmed
			}
		}
	}
	if toUserID <= 0 && externalTo == nil {
		_, _ = io.WriteString(sess, "\r\nCould not determine reply target. Press any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	targetLabel := "local caller"
	if externalTo != nil {
		targetLabel = *externalTo
	}
	writeReplyDesk := func(note string) {
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Mail Reply", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.RenderMailCompose(renderWidth, "Private Mail Reply", targetLabel, subject, "normal", "", note), ansiEnabled, encoding)
	}
	writeReplyDesk("Your reply is added above a quoted copy of the original message.")
	_, _ = io.WriteString(sess, "Subject ["+subject+"]: ")
	override, err := readLine(reader, 120)
	if err != nil {
		return err
	}
	touch()
	override = strings.TrimSpace(override)
	if override != "" {
		subject = override
	}
	writeReplyDesk("Type your reply below. The quoted original will be attached when you send.")
	_, _ = io.WriteString(sess, "\r\nEnter reply body, end with '.' on a line by itself.\r\n")
	replyBody, err := readMessageBody(sess, reader, 80, 4096)
	if err != nil {
		return err
	}
	touch()
	replyBody = strings.TrimSpace(replyBody)
	if replyBody == "" {
		_, _ = io.WriteString(sess, "\r\nReply body is required. Press any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	replyBody = strings.TrimSpace(replyBody + "\n\n" + quoteMessage(source.Body))
	reply := &domain.PrivateMail{
		FromUserID: currentUser.ID,
		ToUserID:   toUserID,
		ExternalTo: externalTo,
		Subject:    subject,
		Body:       replyBody,
	}
	if err := s.mail.CreateMail(reply); err != nil {
		_, _ = io.WriteString(sess, "\r\nCould not send reply: "+err.Error()+"\r\nPress any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	s.publishEvent("mail.reply", map[string]string{
		"user": handle,
		"id":   strconv.FormatInt(reply.ID, 10),
	})
	_, _ = io.WriteString(sess, "\r\nReply sent. Press any key.")
	_, _ = readKey(reader)
	touch()
	return nil
}

func (s *Server) runMailTemplateDesk(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) (*mailflow.Template, error) {
	if touch == nil {
		touch = func() {}
	}
	for {
		templates := mailflow.LoadTemplates(s.admin, handle)
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Reply Kits", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		var body strings.Builder
		body.WriteString("Saved Reply Kits\r\n\r\n")
		body.WriteString("Keep your strongest recurring replies ready for reuse.\r\n\r\n")
		body.WriteString("Slot  Name                 Subject                           Used\r\n")
		body.WriteString("----  -------------------  --------------------------------  ----\r\n")
		if len(templates) == 0 {
			body.WriteString("(none yet)\r\n")
		} else {
			for idx, row := range templates {
				body.WriteString(fmt.Sprintf("%-4d  %-19s  %-32s  %4d\r\n", idx+1, clampForTTY(row.Name, 19), clampForTTY(row.Subject, 32), row.UseCount))
			}
		}
		body.WriteString("\r\nCommands: N new  E<ID> edit  D<ID> delete  U<ID> use for compose  Q quit\r\n")
		renderFrame(sess, termWidth, renderWidth, body.String(), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		choice, err := readLine(reader, 24)
		if err != nil {
			return nil, err
		}
		touch()
		trimmed := strings.TrimSpace(choice)
		upper := strings.ToUpper(trimmed)
		switch {
		case upper == "Q":
			return nil, nil
		case upper == "N":
			if err := s.editMailTemplate(sess, reader, termWidth, renderWidth, handle, th, ansiEnabled, encoding, time24h, nodeLabel, touch, nil); err != nil {
				return nil, err
			}
		case strings.HasPrefix(upper, "E"):
			row, ok := templateBySlot(templates, trimmed[1:])
			if !ok {
				_, _ = io.WriteString(sess, "\r\nTemplate not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			copyRow := row
			if err := s.editMailTemplate(sess, reader, termWidth, renderWidth, handle, th, ansiEnabled, encoding, time24h, nodeLabel, touch, &copyRow); err != nil {
				return nil, err
			}
		case strings.HasPrefix(upper, "D"):
			row, ok := templateBySlot(templates, trimmed[1:])
			if !ok {
				_, _ = io.WriteString(sess, "\r\nTemplate not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if _, err := mailflow.DeleteTemplate(s.admin, handle, row.ID); err != nil {
				_, _ = io.WriteString(sess, "\r\nDelete failed: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
			}
		case strings.HasPrefix(upper, "U"):
			row, ok := templateBySlot(templates, trimmed[1:])
			if !ok {
				_, _ = io.WriteString(sess, "\r\nTemplate not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			return &row, nil
		default:
			_, _ = io.WriteString(sess, "\r\nUnknown command. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func templateBySlot(rows []mailflow.Template, raw string) (mailflow.Template, bool) {
	slot, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || slot <= 0 || slot > len(rows) {
		return mailflow.Template{}, false
	}
	return rows[slot-1], true
}

func (s *Server) editMailTemplate(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func(), existing *mailflow.Template) error {
	if touch == nil {
		touch = func() {}
	}
	name := ""
	subject := ""
	body := ""
	urgency := "normal"
	id := ""
	if existing != nil {
		id = existing.ID
		name = existing.Name
		subject = existing.Subject
		body = existing.Body
		if existing.Urgency != "" {
			urgency = normalizeMailUrgencySSH(existing.Urgency)
		}
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Reply Kit Editor", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	if name != "" {
		_, _ = io.WriteString(sess, "Kit name ["+name+"]: ")
	} else {
		_, _ = io.WriteString(sess, "Kit name: ")
	}
	value, err := readLine(reader, 64)
	if err != nil {
		return err
	}
	touch()
	value = strings.TrimSpace(value)
	if value != "" {
		name = value
	}
	if subject != "" {
		_, _ = io.WriteString(sess, "Subject ["+subject+"]: ")
	} else {
		_, _ = io.WriteString(sess, "Subject: ")
	}
	value, err = readLine(reader, 120)
	if err != nil {
		return err
	}
	touch()
	value = strings.TrimSpace(value)
	if value != "" {
		subject = value
	}
	_, _ = io.WriteString(sess, "Urgency ["+urgency+"]: ")
	value, err = readLine(reader, 16)
	if err != nil {
		return err
	}
	touch()
	value = strings.TrimSpace(value)
	if value != "" {
		urgency = normalizeMailUrgencySSH(value)
	}
	_, _ = io.WriteString(sess, "\r\nTemplate body; use /preview, /del, '.' to save.\r\n")
	bodyValue, err := readMessageBodySeeded(sess, reader, 80, 4096, body)
	if err != nil {
		return err
	}
	touch()
	if _, err := mailflow.UpsertTemplate(s.admin, handle, mailflow.Template{ID: id, Name: name, Subject: subject, Body: bodyValue, Urgency: urgency}); err != nil {
		_, _ = io.WriteString(sess, "\r\nCould not save reply kit: "+err.Error()+"\r\nPress any key.")
		_, _ = readKey(reader)
		touch()
		return nil
	}
	_, _ = io.WriteString(sess, "\r\nReply kit saved. Press any key.")
	_, _ = readKey(reader)
	touch()
	return nil
}

func readMessageBodySeeded(out io.Writer, reader *bufio.Reader, maxLines, maxChars int, seed string) (string, error) {
	if maxLines <= 0 {
		maxLines = 80
	}
	if maxChars <= 0 {
		maxChars = 4096
	}
	var lines []string
	total := 0
	for _, line := range strings.Split(strings.ReplaceAll(strings.TrimSpace(seed), "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
		total += len(line)
	}
	for i := 0; i < maxLines; i++ {
		if out != nil {
			_, _ = io.WriteString(out, "Body> ")
		}
		line, err := readLine(reader, 512)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "." {
			break
		}
		switch strings.ToLower(trimmed) {
		case "/help":
			if out != nil {
				_, _ = io.WriteString(out, "\r\nCompose helpers: /preview, /del, /help, '.' to send.\r\n")
			}
			continue
		case "/del":
			if len(lines) > 0 {
				lines = lines[:len(lines)-1]
				if out != nil {
					_, _ = io.WriteString(out, "\r\nRemoved last line.\r\n")
				}
			}
			continue
		case "/preview":
			if out != nil {
				preview := strings.Join(lines, "\r\n")
				if strings.TrimSpace(preview) == "" {
					preview = "(draft is empty)"
				}
				_, _ = io.WriteString(out, "\r\n--- draft preview ---\r\n"+preview+"\r\n--- end preview ---\r\n")
			}
			continue
		}
		total += len(line)
		if total > maxChars {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func normalizeMailUrgencySSH(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal", "fyi", "urgent", "asap":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "normal"
	}
}

func applyMailUrgencySSH(subject, urgency string) string {
	subject = strings.TrimSpace(subject)
	urgency = normalizeMailUrgencySSH(urgency)
	prefixes := []string{"[FYI]", "[URGENT]", "[ASAP]"}
	for _, prefix := range prefixes {
		subject = strings.TrimSpace(strings.TrimPrefix(subject, prefix))
	}
	switch urgency {
	case "fyi":
		return "[FYI] " + subject
	case "urgent":
		return "[URGENT] " + subject
	case "asap":
		return "[ASAP] " + subject
	default:
		return subject
	}
}

func defaultIfBlankSSH(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
