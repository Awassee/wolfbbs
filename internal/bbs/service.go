package bbs

import (
	"errors"
	"strings"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

type Service struct {
	boards   repository.BoardRepository
	messages repository.MessageRepository
}

func NewService(boards repository.BoardRepository, messages repository.MessageRepository) *Service {
	return &Service{boards: boards, messages: messages}
}

func (s *Service) ListBoards() ([]domain.Board, error) { return s.boards.List() }

func (s *Service) ListMessages(boardID int64) ([]domain.Message, error) {
	return s.messages.ListByBoard(boardID)
}

func (s *Service) Post(boardID, authorID int64, subject, body string) error {
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if subject == "" || body == "" {
		return errors.New("subject and body are required")
	}
	return s.messages.CreateMessage(&domain.Message{BoardID: boardID, AuthorID: authorID, Subject: subject, Body: body})
}
