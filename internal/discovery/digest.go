package discovery

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

type Item struct {
	Kind      string
	BoardID   int64
	MessageID int64
	When      time.Time
	Line      string
}

type Result struct {
	Since time.Time
	Items []Item
}

func BuildSinceLastCall(
	boardRepo repository.BoardRepository,
	msgRepo repository.MessageRepository,
	mailRepo repository.PrivateMailRepository,
	user *domain.User,
	maxItems int,
) (Result, error) {
	if user == nil {
		return Result{}, fmt.Errorf("user is required")
	}
	if boardRepo == nil || msgRepo == nil {
		return Result{}, fmt.Errorf("board/message repositories are required")
	}
	if maxItems <= 0 {
		maxItems = 12
	}
	if maxItems > 30 {
		maxItems = 30
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	if user.LastLoginAt != nil && !user.LastLoginAt.IsZero() {
		since = user.LastLoginAt.UTC()
	}

	boards, err := boardRepo.List()
	if err != nil {
		return Result{}, err
	}

	boardNames := map[int64]string{}
	boardPointers := map[int64]int64{}
	for _, board := range boards {
		boardNames[board.ID] = strings.TrimSpace(board.Name)
		if ptr, ptrErr := msgRepo.GetPointer(user.ID, board.ID); ptrErr == nil && ptr != nil {
			boardPointers[board.ID] = ptr.LastReadID
		}
	}

	authored := map[int64]struct{}{}
	allByBoard := map[int64][]domain.Message{}
	for _, board := range boards {
		msgs, listErr := msgRepo.ListByBoard(board.ID)
		if listErr != nil {
			continue
		}
		allByBoard[board.ID] = msgs
		for _, msg := range msgs {
			if msg.AuthorID == user.ID {
				authored[msg.ID] = struct{}{}
			}
		}
	}

	handleLower := strings.ToLower(strings.TrimSpace(user.Handle))
	seen := map[string]struct{}{}
	out := make([]Item, 0, maxItems)
	boardActivityCount := map[int64]int{}

	push := func(item Item) {
		key := item.Kind + "|" + item.Line
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}

	for boardID, msgs := range allByBoard {
		pointerID := boardPointers[boardID]
		for _, msg := range msgs {
			isNew := false
			if pointerID > 0 {
				isNew = msg.ID > pointerID
			} else {
				isNew = !msg.CreatedAt.UTC().Before(since)
			}
			if !isNew || msg.AuthorID == user.ID {
				continue
			}
			subject := cleanText(msg.Subject, 42)
			boardName := fallback(boardNames[boardID], fmt.Sprintf("Board %d", boardID))
			stamp := msg.CreatedAt.Local().Format("01-02 15:04")

			combined := strings.ToLower(msg.Subject + "\n" + msg.Body)
			if handleLower != "" && strings.Contains(combined, handleLower) {
				push(Item{
					Kind:      "mention",
					BoardID:   boardID,
					MessageID: msg.ID,
					When:      msg.CreatedAt.UTC(),
					Line:      fmt.Sprintf("%s mention in %s: %s", stamp, boardName, subject),
				})
			}

			if msg.ParentID > 0 {
				if _, ok := authored[msg.ParentID]; ok {
					push(Item{
						Kind:      "reply",
						BoardID:   boardID,
						MessageID: msg.ID,
						When:      msg.CreatedAt.UTC(),
						Line:      fmt.Sprintf("%s reply in %s: %s", stamp, boardName, subject),
					})
				}
			}

			// Keep this explicit and transparent: no hidden ranking.
			if boardActivityCount[boardID] < 2 {
				push(Item{
					Kind:      "board",
					BoardID:   boardID,
					MessageID: msg.ID,
					When:      msg.CreatedAt.UTC(),
					Line:      fmt.Sprintf("%s new in %s: %s", stamp, boardName, subject),
				})
				boardActivityCount[boardID]++
			}
		}
	}

	if mailRepo != nil {
		inbox, _ := mailRepo.ListInbox(user.ID, 100)
		for _, row := range inbox {
			if row.CreatedAt.UTC().Before(since) {
				continue
			}
			subject := cleanText(row.Subject, 42)
			stamp := row.CreatedAt.Local().Format("01-02 15:04")
			state := "new"
			if row.ReadAt != nil {
				state = "read"
			}
			push(Item{
				Kind: "mail",
				When: row.CreatedAt.UTC(),
				Line: fmt.Sprintf("%s mail (%s): %s", stamp, state, subject),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].When.Equal(out[j].When) {
			return out[i].Line < out[j].Line
		}
		return out[i].When.After(out[j].When)
	})
	if len(out) > maxItems {
		out = out[:maxItems]
	}
	return Result{Since: since, Items: out}, nil
}

func BuildAICatchUpLine(items []Item) string {
	if len(items) == 0 {
		return ""
	}
	reply := 0
	mention := 0
	mail := 0
	board := 0
	for _, item := range items {
		switch item.Kind {
		case "reply":
			reply++
		case "mention":
			mention++
		case "mail":
			mail++
		case "board":
			board++
		}
	}
	return fmt.Sprintf("[AI-LABEL] Catch-up: %d replies, %d mentions, %d mail, %d board updates.", reply, mention, mail, board)
}

func cleanText(value string, limit int) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return strings.TrimSpace(value)
	}
	r := []rune(strings.TrimSpace(value))
	if len(r) <= limit {
		return string(r)
	}
	if limit <= 1 {
		return string(r[:limit])
	}
	return string(r[:limit-1]) + "…"
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return strings.TrimSpace(value)
}
