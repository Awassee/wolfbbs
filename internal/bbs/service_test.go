package bbs_test

import (
	"testing"

	"wolfbbs/internal/bbs"
<<<<<<< ours
	"wolfbbs/internal/domain"
=======
>>>>>>> theirs
	"wolfbbs/internal/repository"
)

func TestPostAndListMessages(t *testing.T) {
<<<<<<< ours
	boardRepo := repository.NewInMemoryBoardRepository()
	messages := repository.NewInMemoryMessageRepository()
	svc := bbs.NewService(boardRepo, messages)
	if err := boardRepo.Create(&domain.Board{Name: "General", Description: "Main", CreatedBy: 1}); err != nil {
		t.Fatalf("create board: %v", err)
	}
	boards, _ := svc.ListBoards()
	if len(boards) == 0 {
		t.Fatal("expected boards")
=======
	svc := bbs.NewService(repository.NewInMemoryBoardRepository())
	boards, _ := svc.ListBoards()
	if len(boards) == 0 {
		t.Fatal("expected seeded boards")
>>>>>>> theirs
	}
	if err := svc.Post(boards[0].ID, 1, "Hello", "WolfBBS post body"); err != nil {
		t.Fatalf("post failed: %v", err)
	}
	msgs, err := svc.ListMessages(boards[0].ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}
